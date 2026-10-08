package auth

import (
	"sync"
	"time"
)

const (
	maxLoginFailures = 10
	loginWindow      = 15 * time.Minute
)

// LoginLimiter ограничивает число неудачных попыток входа на один логин.
// Хранится в памяти процесса: при одном экземпляре сервера этого достаточно.
type LoginLimiter struct {
	mu       sync.Mutex
	failures map[string][]time.Time
}

func NewLoginLimiter() *LoginLimiter {
	return &LoginLimiter{failures: make(map[string][]time.Time)}
}

func (l *LoginLimiter) recent(key string, now time.Time) []time.Time {
	kept := l.failures[key][:0]
	for _, t := range l.failures[key] {
		if now.Sub(t) < loginWindow {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		delete(l.failures, key)
		return nil
	}
	l.failures[key] = kept
	return kept
}

func (l *LoginLimiter) Blocked(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.recent(key, time.Now())) >= maxLoginFailures
}

func (l *LoginLimiter) Fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	l.failures[key] = append(l.recent(key, now), now)
}

func (l *LoginLimiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.failures, key)
}
