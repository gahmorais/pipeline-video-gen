package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/lib/pq"

	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/ports"
)

// Listener acorda workers via LISTEN/NOTIFY. Notificações são apenas uma dica:
// o worker sempre faz Claim (e também faz polling periódico), então perder
// uma notificação nunca perde trabalho.
type Listener struct {
	l      *pq.Listener
	wakeup chan struct{}
}

// NewListener assina o canal do tipo de tarefa. A conexão se reconecta sozinha.
func NewListener(dsn string, kind ports.TaskKind, logger *slog.Logger) (*Listener, error) {
	pl := pq.NewListener(dsn, 1*time.Second, 30*time.Second, func(ev pq.ListenerEventType, err error) {
		if err != nil {
			logger.Warn("listener postgres", "event", ev, "err", err)
		}
	})
	if err := pl.Listen(NotifyChannel(kind)); err != nil {
		_ = pl.Close()
		return nil, fmt.Errorf("LISTEN %s: %w", NotifyChannel(kind), err)
	}
	l := &Listener{l: pl, wakeup: make(chan struct{}, 1)}
	go l.pump()
	return l, nil
}

func (l *Listener) pump() {
	for range l.l.Notify {
		// nil também chega após reconexão: acordar é o comportamento correto.
		select {
		case l.wakeup <- struct{}{}:
		default: // já há um wakeup pendente
		}
	}
	close(l.wakeup)
}

// Wait bloqueia até uma notificação, o timeout de polling ou o cancelamento do contexto.
func (l *Listener) Wait(ctx context.Context, pollInterval time.Duration) error {
	timer := time.NewTimer(pollInterval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-l.wakeup:
		return nil
	case <-timer.C:
		return nil
	}
}

func (l *Listener) Close() error { return l.l.Close() }
