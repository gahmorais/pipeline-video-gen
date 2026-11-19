// Package memory fornece uma UnitOfWork em memória com semântica transacional.
// Usada em testes de adapters (HTTP) e para rodar a API sem Postgres em desenvolvimento.
package memory

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sync"

	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/domain"
	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/ports"
)

// Store serializa transações: fn opera sobre uma cópia, publicada apenas se fn não falhar.
type Store struct {
	mu    sync.Mutex
	jobs  map[domain.JobID]domain.Snapshot
	queue []ports.Task
}

var _ ports.UnitOfWork = (*Store)(nil)

func NewStore() *Store { return &Store{jobs: map[domain.JobID]domain.Snapshot{}} }

func (s *Store) Do(ctx context.Context, fn func(context.Context, ports.Repositories) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx := &tx{jobs: maps.Clone(s.jobs), queue: slices.Clone(s.queue)}
	if err := fn(ctx, ports.Repositories{Jobs: tx, Queue: tx}); err != nil {
		return err
	}
	s.jobs, s.queue = tx.jobs, tx.queue
	return nil
}

// Tasks devolve as tarefas enfileiradas (inspeção em testes).
func (s *Store) Tasks() []ports.Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.queue)
}

type tx struct {
	jobs  map[domain.JobID]domain.Snapshot
	queue []ports.Task
}

func (t *tx) Get(_ context.Context, id domain.JobID) (*domain.VideoJob, error) {
	snap, ok := t.jobs[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", domain.ErrNotFound, id)
	}
	return domain.Rehydrate(snap)
}

func (t *tx) Create(_ context.Context, j *domain.VideoJob) error {
	if _, ok := t.jobs[j.ID()]; ok {
		return fmt.Errorf("job %s já existe", j.ID())
	}
	j.SetVersion(1)
	t.jobs[j.ID()] = j.Snapshot()
	return nil
}

func (t *tx) Update(_ context.Context, j *domain.VideoJob) error {
	cur, ok := t.jobs[j.ID()]
	if !ok {
		return fmt.Errorf("%w: %s", domain.ErrNotFound, j.ID())
	}
	if cur.Version != j.Version() {
		return domain.ErrConcurrentUpdate
	}
	j.SetVersion(j.Version() + 1)
	t.jobs[j.ID()] = j.Snapshot()
	return nil
}

func (t *tx) Enqueue(_ context.Context, task ports.Task) error {
	t.queue = append(t.queue, task)
	return nil
}
