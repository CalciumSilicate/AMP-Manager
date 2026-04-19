package service

import (
	"sync"
	"time"
)

var statsLocationCache struct {
	mu         sync.RWMutex
	initialized bool
	location   *time.Location
}

func loadCachedStatsLocation() *time.Location {
	statsLocationCache.mu.RLock()
	if statsLocationCache.initialized && statsLocationCache.location != nil {
		location := statsLocationCache.location
		statsLocationCache.mu.RUnlock()
		return location
	}
	statsLocationCache.mu.RUnlock()

	return RefreshStatsLocationCache()
}

func CurrentStatsLocation() *time.Location {
	return loadCachedStatsLocation()
}

func RefreshStatsLocationCache() *time.Location {
	location, err := NewSystemConfigService().GetSiteLocation()
	if err != nil || location == nil {
		location = time.UTC
	}

	statsLocationCache.mu.Lock()
	statsLocationCache.initialized = true
	statsLocationCache.location = location
	statsLocationCache.mu.Unlock()
	return location
}
