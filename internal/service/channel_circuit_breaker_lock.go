package service

import "sync"

var channelCircuitBreakerLocks sync.Map

func WithChannelCircuitBreakerLock(channelID string, fn func()) {
	if channelID == "" {
		fn()
		return
	}

	lockAny, _ := channelCircuitBreakerLocks.LoadOrStore(channelID, &sync.Mutex{})
	lock := lockAny.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()
	fn()
}
