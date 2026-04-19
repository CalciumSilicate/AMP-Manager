package invalidation

import (
	"context"
	"sync"
	"time"

	"ampmanager/internal/config"

	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"
)

const (
	ChannelErrorRulesUpdated          = "ampmanager:cache:error_rules:updated"
	ChannelRequestFiltersUpdated      = "ampmanager:cache:request_filters:updated"
	ChannelRetryConfigUpdated         = "ampmanager:runtime:retry_config:updated"
	ChannelRequestPayloadLimitUpdated = "ampmanager:runtime:request_payload_limit:updated"
	ChannelRequestDetailConfigUpdated = "ampmanager:runtime:request_detail_config:updated"
	ChannelTimeoutConfigUpdated       = "ampmanager:runtime:timeout_config:updated"
	ChannelSessionStickyConfigUpdated = "ampmanager:runtime:session_sticky_config:updated"
	ChannelBillingRuntimeUpdated      = "ampmanager:runtime:billing_runtime:updated"
	ChannelUserPanelRateLimitUpdated  = "ampmanager:runtime:user_panel_rate_limit:updated"
	ChannelSiteConfigUpdated          = "ampmanager:runtime:site_config:updated"
	ChannelChannelsUpdated            = "ampmanager:cache:channels:updated"
	ChannelModelMetadataUpdated       = "ampmanager:cache:model_metadata:updated"
	ChannelPriceStoreUpdated          = "ampmanager:cache:price_store:updated"
)

type callback func()

var (
	mu           sync.Mutex
	localSubs    = make(map[string]map[int]callback)
	nextSubID    int
	redisClients = make(map[string]*redis.Client)
	redisStarted = make(map[string]bool)
)

func Subscribe(channel string, fn func()) func() {
	if fn == nil || channel == "" {
		return func() {}
	}

	mu.Lock()
	id := nextSubID
	nextSubID++
	if localSubs[channel] == nil {
		localSubs[channel] = make(map[int]callback)
	}
	localSubs[channel][id] = fn
	mu.Unlock()

	ensureRedisSubscriber(channel)

	return func() {
		mu.Lock()
		defer mu.Unlock()
		if subs := localSubs[channel]; subs != nil {
			delete(subs, id)
			if len(subs) == 0 {
				delete(localSubs, channel)
			}
		}
	}
}

func Emit(channel string) {
	mu.Lock()
	snapshot := make([]callback, 0, len(localSubs[channel]))
	for _, fn := range localSubs[channel] {
		snapshot = append(snapshot, fn)
	}
	mu.Unlock()

	for _, fn := range snapshot {
		func(run func()) {
			defer func() {
				if recovered := recover(); recovered != nil {
					log.Warnf("invalidation: callback panic on channel %s: %v", channel, recovered)
				}
			}()
			run()
		}(fn)
	}
}

func Publish(channel string) {
	Emit(channel)

	client := redisPublisher()
	if client == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Publish(ctx, channel, time.Now().UTC().Format(time.RFC3339Nano)).Err(); err != nil {
		log.Warnf("invalidation: publish failed on channel %s: %v", channel, err)
	}
}

func redisPublisher() *redis.Client {
	cfg := config.Get()
	if cfg == nil || cfg.RedisURL == "" {
		return nil
	}

	mu.Lock()
	defer mu.Unlock()
	client := redisClients["publisher"]
	if client != nil {
		return client
	}

	options, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		log.Warnf("invalidation: invalid REDIS_URL for publisher: %v", err)
		return nil
	}
	client = redis.NewClient(options)
	redisClients["publisher"] = client
	return client
}

func ensureRedisSubscriber(channel string) {
	cfg := config.Get()
	if cfg == nil || cfg.RedisURL == "" || channel == "" {
		return
	}

	mu.Lock()
	if redisStarted[channel] {
		mu.Unlock()
		return
	}
	redisStarted[channel] = true
	mu.Unlock()

	go runSubscriber(channel, cfg.RedisURL)
}

func runSubscriber(channel, redisURL string) {
	options, err := redis.ParseURL(redisURL)
	if err != nil {
		log.Warnf("invalidation: invalid REDIS_URL for subscriber %s: %v", channel, err)
		return
	}

	for {
		client := redis.NewClient(options)
		pubsub := client.Subscribe(context.Background(), channel)
		subCtx := pubsub.Channel()

		for msg := range subCtx {
			if msg == nil {
				continue
			}
			Emit(channel)
		}

		_ = pubsub.Close()
		_ = client.Close()
		time.Sleep(2 * time.Second)
	}
}
