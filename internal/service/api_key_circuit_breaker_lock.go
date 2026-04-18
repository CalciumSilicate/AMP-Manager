package service

import "sync"

var apiKeyCircuitBreakerLocks sync.Map

func WithAPIKeyCircuitBreakerLock(keyID string, fn func()) {
	if keyID == "" {
		fn()
		return
	}

	lockAny, _ := apiKeyCircuitBreakerLocks.LoadOrStore(keyID, &sync.Mutex{})
	lock := lockAny.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()
	fn()
}
