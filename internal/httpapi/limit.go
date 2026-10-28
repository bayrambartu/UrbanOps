package httpapi

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// A bounded, process-local limiter for the single-instance prototype.
// Distributed limits can later be backed by Redis.
func limitLogin(next http.HandlerFunc) http.HandlerFunc {
	var mu sync.Mutex
	counts := map[string]int{}
	window := time.Now()
	return func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		mu.Lock()
		if time.Since(window) >= time.Minute {
			counts = map[string]int{}
			window = time.Now()
		}
		blocked := counts[host] >= 10 || len(counts) >= 10000
		if !blocked {
			counts[host]++
		}
		mu.Unlock()
		if blocked {
			w.Header().Set("Retry-After", "60")
			problem(w, 429, "Çok fazla giriş denemesi; bir dakika sonra tekrar deneyin")
			return
		}
		next(w, r)
	}
}
