package httpapi

import (
	"fmt"
	"math"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// RateLimit configura um token bucket: Burst requisições imediatas e
// reposição de PerMinute por minuto, por IP de origem.
type RateLimit struct {
	PerMinute float64
	Burst     float64
}

// rateLimiter limita por (rota, IP). Em memória: adequado a uma instância (cenário local).
// Não confia em X-Forwarded-For (sem proxy reverso na frente).
type rateLimiter struct {
	mu      sync.Mutex
	limit   RateLimit
	buckets map[string]*bucket
	now     func() time.Time
}

type bucket struct {
	tokens float64
	last   time.Time
}

func newRateLimiter(limit RateLimit, now func() time.Time) *rateLimiter {
	return &rateLimiter{limit: limit, buckets: map[string]*bucket{}, now: now}
}

// allow consome um token; se não houver, devolve o tempo até o próximo.
func (l *rateLimiter) allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.limit.Burst, last: now}
		l.buckets[key] = b
		l.evictStale(now)
	}
	ratePerSec := l.limit.PerMinute / 60
	b.tokens = math.Min(l.limit.Burst, b.tokens+now.Sub(b.last).Seconds()*ratePerSec)
	b.last = now
	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	return false, time.Duration((1 - b.tokens) / ratePerSec * float64(time.Second))
}

// evictStale remove buckets cheios (inativos) para limitar memória.
func (l *rateLimiter) evictStale(now time.Time) {
	if len(l.buckets) < 1024 {
		return
	}
	full := time.Duration(l.limit.Burst / (l.limit.PerMinute / 60) * float64(time.Second))
	for k, b := range l.buckets {
		if now.Sub(b.last) > full {
			delete(l.buckets, k)
		}
	}
}

func (s *Server) rateLimited(limiter *rateLimiter, next http.HandlerFunc) http.HandlerFunc {
	if limiter == nil {
		return next
	}
	return func(w http.ResponseWriter, r *http.Request) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}
		ok, retryAfter := limiter.allow(r.Pattern + "|" + ip)
		if !ok {
			secs := int(math.Ceil(retryAfter.Seconds()))
			w.Header().Set("Retry-After", strconv.Itoa(secs))
			writeProblem(w, problem{
				Type: "rate-limited", Title: "Muitas requisições", Status: http.StatusTooManyRequests,
				Detail: fmt.Sprintf("tente novamente em %d s", secs),
			})
			return
		}
		next(w, r)
	}
}
