package service

import (
	"errors"
	"math"
	"time"

	"ampmanager/internal/model"
	"ampmanager/internal/repository"
)

var (
	ErrUnknownLimitType = errors.New("未知的限制类型")
)

var farFuture = time.Date(2099, 12, 31, 23, 59, 59, 0, time.UTC)

type QuotaService struct {
	eventRepo repository.BillingEventRepositoryInterface
	subRepo   repository.UserSubscriptionRepositoryInterface
	planRepo  repository.SubscriptionPlanRepositoryInterface
	configSvc *SystemConfigService
}

func NewQuotaService() *QuotaService {
	return &QuotaService{
		eventRepo: repository.NewBillingEventRepository(),
		subRepo:   repository.NewUserSubscriptionRepository(),
		planRepo:  repository.NewSubscriptionPlanRepository(),
		configSvc: NewSystemConfigService(),
	}
}

func NewQuotaServiceWithRepo(
	eventRepo repository.BillingEventRepositoryInterface,
	subRepo repository.UserSubscriptionRepositoryInterface,
	planRepo repository.SubscriptionPlanRepositoryInterface,
) *QuotaService {
	return &QuotaService{
		eventRepo: eventRepo,
		subRepo:   subRepo,
		planRepo:  planRepo,
		configSvc: NewSystemConfigService(),
	}
}

func getStartOfWeek(now time.Time) time.Time {
	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7
	}

	return time.Date(now.Year(), now.Month(), now.Day()-(weekday-1), 0, 0, 0, 0, now.Location())
}

func getDailyFixedWindowBounds(now time.Time, fixedResetTime *string) (time.Time, time.Time, error) {
	minutes := 0
	if fixedResetTime != nil {
		parsed, err := model.ParseFixedResetTime(*fixedResetTime)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		minutes = parsed
	}

	resetHour := minutes / 60
	resetMinute := minutes % 60
	todayReset := time.Date(now.Year(), now.Month(), now.Day(), resetHour, resetMinute, 0, 0, now.Location())
	start := todayReset
	if now.Before(todayReset) {
		yesterday := now.AddDate(0, 0, -1)
		start = time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), resetHour, resetMinute, 0, 0, now.Location())
	}

	nextDay := start.AddDate(0, 0, 1)
	end := time.Date(nextDay.Year(), nextDay.Month(), nextDay.Day(), resetHour, resetMinute, 0, 0, now.Location())
	return start, end, nil
}

func GetWindowBounds(limit model.SubscriptionPlanLimit, now time.Time, subscriptionStartsAt time.Time, location *time.Location) (start, end time.Time, err error) {
	if location == nil {
		location = time.UTC
	}

	localNow := now.In(location)

	switch limit.LimitType {
	case model.LimitTypeDaily:
		if limit.WindowMode == model.WindowModeFixed {
			start, end, err = getDailyFixedWindowBounds(localNow, limit.FixedResetTime)
			if err != nil {
				return time.Time{}, time.Time{}, err
			}
		} else {
			start = now.Add(-24 * time.Hour)
			end = now
		}
	case model.LimitTypeWeekly:
		if limit.WindowMode == model.WindowModeFixed {
			start = getStartOfWeek(localNow)
			end = start.AddDate(0, 0, 7)
		} else {
			start = now.AddDate(0, 0, -7)
			end = now
		}
	case model.LimitTypeMonthly:
		if limit.WindowMode == model.WindowModeFixed {
			start = time.Date(localNow.Year(), localNow.Month(), 1, 0, 0, 0, 0, location)
			end = start.AddDate(0, 1, 0)
		} else {
			start = now.AddDate(0, -1, 0)
			end = now
		}
	case model.LimitTypeRolling5h:
		const windowSec int64 = 18000
		if limit.WindowMode == model.WindowModeFixed {
			unix := now.Unix()
			floorUnix := (unix / windowSec) * windowSec
			start = time.Unix(floorUnix, 0).UTC()
			end = start.Add(5 * time.Hour)
		} else {
			start = now.Add(-5 * time.Hour)
			end = now
		}
	case model.LimitTypeTotal:
		start = subscriptionStartsAt
		end = farFuture
	default:
		return time.Time{}, time.Time{}, ErrUnknownLimitType
	}

	if limit.WindowMode == model.WindowModeFixed && limit.LimitType != model.LimitTypeRolling5h && limit.LimitType != model.LimitTypeTotal {
		return start.UTC(), end.UTC(), nil
	}

	return start, end, nil
}

func (s *QuotaService) getSiteLocation() (*time.Location, error) {
	if s.configSvc == nil {
		return time.LoadLocation(defaultSiteTimeZone)
	}
	return s.configSvc.GetSiteLocation()
}

func (s *QuotaService) GetSubscriptionRemaining(userID string) (int64, []model.WindowRemaining, error) {
	sub, err := s.subRepo.GetActiveByUserID(userID)
	if err != nil {
		return 0, nil, err
	}
	if sub == nil {
		return 0, nil, nil
	}

	_, limits, err := s.planRepo.GetByID(sub.PlanID)
	if err != nil {
		return 0, nil, err
	}
	if len(limits) == 0 {
		return 0, nil, nil
	}

	now := time.Now().UTC()
	location, err := s.getSiteLocation()
	if err != nil {
		return 0, nil, err
	}
	windows := make([]model.WindowRemaining, 0, len(limits))
	minRemaining := int64(math.MaxInt64)

	for _, limit := range limits {
		start, end, err := GetWindowBounds(limit, now, sub.StartsAt, location)
		if err != nil {
			return 0, nil, err
		}

		used, err := s.eventRepo.GetUsageInWindow(sub.ID, start, end)
		if err != nil {
			return 0, nil, err
		}

		left := limit.LimitMicros - used
		if left < 0 {
			left = 0
		}

		windows = append(windows, model.WindowRemaining{
			LimitType:   limit.LimitType,
			WindowMode:  limit.WindowMode,
			LimitMicros: limit.LimitMicros,
			UsedMicros:  used,
			LeftMicros:  left,
			WindowStart: start,
			WindowEnd:   end,
		})

		if left < minRemaining {
			minRemaining = left
		}
	}

	if minRemaining == math.MaxInt64 {
		minRemaining = 0
	}

	return minRemaining, windows, nil
}

func (s *QuotaService) GetWindowsDetail(userID string) ([]model.WindowRemaining, error) {
	_, windows, err := s.GetSubscriptionRemaining(userID)
	return windows, err
}
