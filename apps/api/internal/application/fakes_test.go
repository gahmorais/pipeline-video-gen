package application_test

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/domain"
	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/ports"
)

// memStore é uma UnitOfWork em memória com semântica transacional:
// o fn opera sobre uma cópia, que só é publicada se fn não retornar erro.
type memStore struct {
	mu    sync.Mutex
	jobs  map[domain.JobID]domain.Snapshot
	queue []ports.Task

	failEnqueue error
}

func newMemStore() *memStore {
	return &memStore{jobs: map[domain.JobID]domain.Snapshot{}}
}

func (m *memStore) Do(ctx context.Context, fn func(context.Context, ports.Repositories) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	tx := &memTx{store: m, jobs: maps.Clone(m.jobs), queue: slices.Clone(m.queue)}
	if err := fn(ctx, ports.Repositories{Jobs: tx, Queue: tx}); err != nil {
		return err
	}
	m.jobs, m.queue = tx.jobs, tx.queue
	return nil
}

func (m *memStore) tasks() []ports.Task {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.queue)
}

// put grava diretamente um job (setup de teste).
func (m *memStore) put(j *domain.VideoJob) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j.PullEvents()
	m.jobs[j.ID()] = j.Snapshot()
}

func (m *memStore) snapshot(id domain.JobID) domain.Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.jobs[id]
}

type memTx struct {
	store *memStore
	jobs  map[domain.JobID]domain.Snapshot
	queue []ports.Task
}

func (t *memTx) Get(_ context.Context, id domain.JobID) (*domain.VideoJob, error) {
	s, ok := t.jobs[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return domain.Rehydrate(s)
}

func (t *memTx) Create(_ context.Context, j *domain.VideoJob) error {
	if _, exists := t.jobs[j.ID()]; exists {
		return fmt.Errorf("job %s já existe", j.ID())
	}
	j.SetVersion(1)
	t.jobs[j.ID()] = j.Snapshot()
	return nil
}

func (t *memTx) Update(_ context.Context, j *domain.VideoJob) error {
	current, ok := t.jobs[j.ID()]
	if !ok {
		return domain.ErrNotFound
	}
	if current.Version != j.Version() {
		return domain.ErrConcurrentUpdate
	}
	j.SetVersion(j.Version() + 1)
	t.jobs[j.ID()] = j.Snapshot()
	return nil
}

func (t *memTx) Enqueue(_ context.Context, task ports.Task) error {
	if t.store.failEnqueue != nil {
		return t.store.failEnqueue
	}
	t.queue = append(t.queue, task)
	return nil
}

// fakeGenerator devolve uma resposta fixa ou erro.
type fakeGenerator struct {
	out   ports.GenerateOutput
	err   error
	calls []ports.GenerateInput
	// hook executado durante a geração (simula alterações concorrentes).
	during func()
}

func (g *fakeGenerator) Generate(_ context.Context, in ports.GenerateInput) (ports.GenerateOutput, error) {
	g.calls = append(g.calls, in)
	if g.during != nil {
		g.during()
	}
	return g.out, g.err
}

// fakeCatalog: template "promo@1"; storyboard é válido se tiver "valid": true.
type fakeCatalog struct{}

func (fakeCatalog) Latest(id string) (domain.TemplateRef, bool) {
	if id != "promo" {
		return domain.TemplateRef{}, false
	}
	return domain.TemplateRef{ID: "promo", Version: 1}, true
}

func (fakeCatalog) JSONSchema(ref domain.TemplateRef) ([]byte, bool) {
	if ref.ID != "promo" {
		return nil, false
	}
	return []byte(`{"type":"object"}`), true
}

func (fakeCatalog) Validate(_ domain.TemplateRef, raw []byte) ([]ports.ValidationIssue, error) {
	var v struct {
		Valid bool `json:"valid"`
	}
	if err := json.Unmarshal(raw, &v); err != nil || !v.Valid {
		return []ports.ValidationIssue{{Path: "valid", Message: "deve ser true"}}, nil
	}
	return nil, nil
}

type recordingPublisher struct {
	mu     sync.Mutex
	events []domain.Event
}

func (p *recordingPublisher) Publish(_ context.Context, events []domain.Event) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, events...)
}

func (p *recordingPublisher) names() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]string, 0, len(p.events))
	for _, e := range p.events {
		out = append(out, e.EventName())
	}
	return out
}

type seqIDs struct{ n int }

func (s *seqIDs) NewJobID() domain.JobID {
	s.n++
	return domain.JobID(fmt.Sprintf("job-%d", s.n))
}

func (s *seqIDs) NewRenderID() domain.RenderID {
	s.n++
	return domain.RenderID(fmt.Sprintf("render-%d", s.n))
}

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }
