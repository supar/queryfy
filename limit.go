package main

import (
	"net/http"
	"sync"
)

// limiter represents gracefull connection limitation with response.
// To control connections more aggressive http.Connection should be wrapped.
type limiter struct {
	mu  sync.Mutex
	cnt uint8

	lmt uint8
}

func newLimiter(lm uint8) *limiter {
	return &limiter{
		lmt: lm,
	}
}

func (l *limiter) accept() (ok bool) {
	l.mu.Lock()

	if l.cnt < l.lmt {
		l.cnt++
		ok = true
	}

	l.mu.Unlock()
	return
}

func (l *limiter) done() {
	l.mu.Lock()
	if l.cnt > 0 {
		l.cnt--
	}
	l.mu.Unlock()
}

func (l *limiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !l.accept() {
			w.Header().Set("Retry-After", "600")
			w.Header().Set("Connection", "close")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		next.ServeHTTP(w, r)
		l.done()
	})
}
