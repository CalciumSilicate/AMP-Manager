package billingstate

import "github.com/redis/go-redis/v9"

var reserveScript = redis.NewScript(`
local function split_keys(value)
  local keys = {}
  if not value or value == '' then
    return keys
  end
  for key in string.gmatch(value, '([^|]+)') do
    table.insert(keys, key)
  end
  return keys
end

local accountKey = KEYS[1]
local reservationKey = KEYS[2]
local expiryKey = KEYS[3]
local streamKey = KEYS[4]

local estimated = tonumber(ARGV[1]) or 0
local expiresAt = ARGV[2]
local userID = ARGV[3]
local subscriptionID = ARGV[4]
local windowKeyList = ARGV[5]
local windowRefsJSON = ARGV[6]
local nowUnix = ARGV[7]
local requestID = ARGV[8]

if redis.call('EXISTS', accountKey) == 0 then
  return {'MISS'}
end

if redis.call('EXISTS', reservationKey) == 1 then
  return {'EXISTS', redis.call('HGET', reservationKey, 'status') or ''}
end

local windowKeys = split_keys(windowKeyList)
for _, windowKey in ipairs(windowKeys) do
  if redis.call('EXISTS', windowKey) == 0 then
    return {'MISS'}
  end
end

local primary = redis.call('HGET', accountKey, 'primary_source') or 'subscription'
local secondary = redis.call('HGET', accountKey, 'secondary_source') or 'balance'
local balance = tonumber(redis.call('HGET', accountKey, 'balance_micros') or '0')
local reservedSub = 0
local reservedBal = 0
local remaining = estimated
local minWindowRemaining = 0

if #windowKeys > 0 then
  minWindowRemaining = math.huge
  for _, windowKey in ipairs(windowKeys) do
    local current = tonumber(redis.call('HGET', windowKey, 'remaining_micros') or '0')
    if current < minWindowRemaining then
      minWindowRemaining = current
    end
  end
  if minWindowRemaining == math.huge then
    minWindowRemaining = 0
  end
end

local function consume(source)
  if remaining <= 0 then
    return
  end
  if source == 'subscription' then
    local take = math.min(remaining, minWindowRemaining)
    reservedSub = reservedSub + take
    remaining = remaining - take
    minWindowRemaining = minWindowRemaining - take
  elseif source == 'balance' then
    local take = math.min(remaining, balance)
    reservedBal = reservedBal + take
    remaining = remaining - take
    balance = balance - take
  end
end

consume(primary)
consume(secondary)

if remaining > 0 then
  return {'REJECT'}
end

if reservedBal > 0 then
  redis.call('HINCRBY', accountKey, 'balance_micros', -reservedBal)
end
if reservedSub > 0 then
  for _, windowKey in ipairs(windowKeys) do
    redis.call('HINCRBY', windowKey, 'reserved_micros', reservedSub)
    redis.call('HINCRBY', windowKey, 'remaining_micros', -reservedSub)
  end
end

redis.call('HSET', reservationKey,
  'request_id', requestID,
  'user_id', userID,
  'user_subscription_id', subscriptionID,
  'status', 'reserved',
  'estimated_cost_micros', estimated,
  'reserved_subscription_micros', reservedSub,
  'reserved_balance_micros', reservedBal,
  'charged_subscription_micros', 0,
  'charged_balance_micros', 0,
  'window_key_list', windowKeyList,
  'window_refs_json', windowRefsJSON,
  'expires_at_unix', expiresAt,
  'created_at_unix', nowUnix,
  'updated_at_unix', nowUnix
)

redis.call('ZADD', expiryKey, expiresAt, requestID)

redis.call('XADD', streamKey, '*',
  'event_type', 'reserve',
  'request_id', requestID,
  'user_id', userID,
  'user_subscription_id', subscriptionID,
  'estimated_cost_micros', estimated,
  'reserved_subscription_micros', reservedSub,
  'reserved_balance_micros', reservedBal,
  'status', 'reserved',
  'expires_at_unix', expiresAt,
  'window_refs_json', windowRefsJSON
)

return {'OK', tostring(reservedSub), tostring(reservedBal)}
`)

var settleScript = redis.NewScript(`
local function split_keys(value)
  local keys = {}
  if not value or value == '' then
    return keys
  end
  for key in string.gmatch(value, '([^|]+)') do
    table.insert(keys, key)
  end
  return keys
end

local accountKey = KEYS[1]
local reservationKey = KEYS[2]
local expiryKey = KEYS[3]
local streamKey = KEYS[4]
local actual = tonumber(ARGV[1]) or 0
local nowUnix = ARGV[2]

if redis.call('EXISTS', reservationKey) == 0 then
  return {'NOT_FOUND'}
end

local status = redis.call('HGET', reservationKey, 'status') or ''
if status ~= 'reserved' then
  return {'DONE', status}
end

local requestID = redis.call('HGET', reservationKey, 'request_id') or ''
local userID = redis.call('HGET', reservationKey, 'user_id') or ''
local subscriptionID = redis.call('HGET', reservationKey, 'user_subscription_id') or ''
local reservedSub = tonumber(redis.call('HGET', reservationKey, 'reserved_subscription_micros') or '0')
local reservedBal = tonumber(redis.call('HGET', reservationKey, 'reserved_balance_micros') or '0')
local windowKeyList = redis.call('HGET', reservationKey, 'window_key_list') or ''
local windowRefsJSON = redis.call('HGET', reservationKey, 'window_refs_json') or ''
local windowKeys = split_keys(windowKeyList)

local chargedSub = math.min(actual, reservedSub)
local remaining = actual - chargedSub
local chargedBal = math.min(remaining, reservedBal)
remaining = remaining - chargedBal
local releasedSub = reservedSub - chargedSub
local releasedBal = reservedBal - chargedBal
local finalStatus = 'settled'
if remaining > 0 then
  finalStatus = 'under_reserved'
end

if releasedBal > 0 then
  redis.call('HINCRBY', accountKey, 'balance_micros', releasedBal)
end
if reservedSub > 0 then
  for _, windowKey in ipairs(windowKeys) do
    redis.call('HINCRBY', windowKey, 'reserved_micros', -reservedSub)
    if chargedSub > 0 then
      redis.call('HINCRBY', windowKey, 'used_micros', chargedSub)
    end
    if releasedSub > 0 then
      redis.call('HINCRBY', windowKey, 'remaining_micros', releasedSub)
    end
  end
end

redis.call('HSET', reservationKey,
  'status', finalStatus,
  'actual_cost_micros', actual,
  'charged_subscription_micros', chargedSub,
  'charged_balance_micros', chargedBal,
  'released_subscription_micros', releasedSub,
  'released_balance_micros', releasedBal,
  'updated_at_unix', nowUnix
)
redis.call('ZREM', expiryKey, requestID)

redis.call('XADD', streamKey, '*',
  'event_type', 'settle',
  'request_id', requestID,
  'user_id', userID,
  'user_subscription_id', subscriptionID,
  'actual_cost_micros', actual,
  'reserved_subscription_micros', reservedSub,
  'charged_subscription_micros', chargedSub,
  'charged_balance_micros', chargedBal,
  'released_subscription_micros', releasedSub,
  'released_balance_micros', releasedBal,
  'status', finalStatus,
  'window_refs_json', windowRefsJSON
)

return {'OK', finalStatus, tostring(chargedSub), tostring(chargedBal)}
`)

var expireScript = redis.NewScript(`
local function split_keys(value)
  local keys = {}
  if not value or value == '' then
    return keys
  end
  for key in string.gmatch(value, '([^|]+)') do
    table.insert(keys, key)
  end
  return keys
end

local accountKey = KEYS[1]
local reservationKey = KEYS[2]
local expiryKey = KEYS[3]
local streamKey = KEYS[4]
local nowUnix = ARGV[1]

if redis.call('EXISTS', reservationKey) == 0 then
  return {'NOT_FOUND'}
end

local status = redis.call('HGET', reservationKey, 'status') or ''
if status ~= 'reserved' then
  return {'DONE', status}
end

local requestID = redis.call('HGET', reservationKey, 'request_id') or ''
local userID = redis.call('HGET', reservationKey, 'user_id') or ''
local subscriptionID = redis.call('HGET', reservationKey, 'user_subscription_id') or ''
local reservedSub = tonumber(redis.call('HGET', reservationKey, 'reserved_subscription_micros') or '0')
local reservedBal = tonumber(redis.call('HGET', reservationKey, 'reserved_balance_micros') or '0')
local windowKeyList = redis.call('HGET', reservationKey, 'window_key_list') or ''
local windowRefsJSON = redis.call('HGET', reservationKey, 'window_refs_json') or ''
local windowKeys = split_keys(windowKeyList)

if reservedBal > 0 then
  redis.call('HINCRBY', accountKey, 'balance_micros', reservedBal)
end
if reservedSub > 0 then
  for _, windowKey in ipairs(windowKeys) do
    redis.call('HINCRBY', windowKey, 'reserved_micros', -reservedSub)
    redis.call('HINCRBY', windowKey, 'remaining_micros', reservedSub)
  end
end

redis.call('HSET', reservationKey,
  'status', 'expired',
  'released_subscription_micros', reservedSub,
  'released_balance_micros', reservedBal,
  'updated_at_unix', nowUnix
)
redis.call('ZREM', expiryKey, requestID)

redis.call('XADD', streamKey, '*',
  'event_type', 'expire',
  'request_id', requestID,
  'user_id', userID,
  'user_subscription_id', subscriptionID,
  'released_subscription_micros', reservedSub,
  'released_balance_micros', reservedBal,
  'status', 'expired',
  'window_refs_json', windowRefsJSON
)

return {'OK', 'expired'}
`)
