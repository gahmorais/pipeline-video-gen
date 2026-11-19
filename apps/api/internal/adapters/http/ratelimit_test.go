package httpapi

import (
	"testing"
	"time"
)

func TestRateLimiterTokenBucket(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l := newRateLimiter(RateLimit{PerMinute: 60, Burst: 2}, func() time.Time { return now })

	for i := 0; i < 2; i++ {
		if ok, _ := l.allow("ip"); !ok {
			t.Fatalf("requisição %d deveria passar (burst)", i)
		}
	}
	ok, retry := l.allow("ip")
	if ok || retry <= 0 || retry > time.Second {
		t.Fatalf("esperado bloqueio com retry <= 1s, obtido ok=%v retry=%v", ok, retry)
	}
	if ok, _ := l.allow("outro-ip"); !ok {
		t.Fatal("limite é por chave")
	}
	now = now.Add(time.Second) // repõe 1 token (60/min)
	if ok, _ := l.allow("ip"); !ok {
		t.Fatal("token deveria ter sido reposto")
	}
	now = now.Add(time.Hour)
	for i := 0; i < 2; i++ {
		if ok, _ := l.allow("ip"); !ok {
			t.Fatal("bucket não pode passar do burst, mas deve encher até ele")
		}
	}
	if ok, _ := l.allow("ip"); ok {
		t.Fatal("bucket encheu além do burst")
	}
}
