// Package events implementa ports.EventPublisher com um broker em memória
// usado pelo SSE. Adequado para uma única instância da API (cenário local);
// múltiplas instâncias exigiriam fan-out via LISTEN/NOTIFY.
package events

import (
	"context"
	"sync"

	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/domain"
	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/ports"
)

// Broker entrega sinais "o job mudou" aos assinantes daquele job.
// Os sinais são coalescidos (buffer 1): o assinante relê o estado atual,
// então nunca fica para trás nem bloqueia o publicador.
type Broker struct {
	mu   sync.Mutex
	subs map[domain.JobID]map[chan struct{}]struct{}
}

var _ ports.EventPublisher = (*Broker)(nil)

func NewBroker() *Broker {
	return &Broker{subs: map[domain.JobID]map[chan struct{}]struct{}{}}
}

// Subscribe devolve um canal de sinais e a função de cancelamento (idempotente).
func (b *Broker) Subscribe(id domain.JobID) (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	b.mu.Lock()
	if b.subs[id] == nil {
		b.subs[id] = map[chan struct{}]struct{}{}
	}
	b.subs[id][ch] = struct{}{}
	b.mu.Unlock()

	var once sync.Once
	return ch, func() {
		once.Do(func() {
			b.mu.Lock()
			delete(b.subs[id], ch)
			if len(b.subs[id]) == 0 {
				delete(b.subs, id)
			}
			b.mu.Unlock()
		})
	}
}

func (b *Broker) Publish(_ context.Context, evts []domain.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, e := range evts {
		for ch := range b.subs[e.JobID()] {
			select {
			case ch <- struct{}{}:
			default: // já há um sinal pendente
			}
		}
	}
}

// Subscribers informa quantos assinantes um job tem (observabilidade/testes).
func (b *Broker) Subscribers(id domain.JobID) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.subs[id])
}
