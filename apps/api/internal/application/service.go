package application

import (
	"context"
	"fmt"

	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/domain"
	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/ports"
)

// Deps são as dependências dos use cases (injetadas no composition root).
type Deps struct {
	UoW       ports.UnitOfWork
	Generator ports.StoryboardGenerator
	Catalog   ports.TemplateCatalog
	Events    ports.EventPublisher
	IDs       ports.IDGenerator
	Clock     ports.Clock
}

// Service agrupa os use cases da API. Cada método é um use case com
// uma única responsabilidade; o helper mutate concentra o fluxo
// carregar → aplicar comando → salvar (+ enfileirar) → publicar eventos.
type Service struct {
	d Deps
}

func NewService(d Deps) *Service {
	return &Service{d: d}
}

// mutation aplica um comando no agregado e devolve tarefas a enfileirar
// na mesma transação. Retornar changed=false evita escrita desnecessária.
type mutation func(job *domain.VideoJob) (changed bool, tasks []ports.Task, err error)

func (s *Service) mutate(ctx context.Context, id domain.JobID, m mutation) (*domain.VideoJob, error) {
	var (
		job    *domain.VideoJob
		events []domain.Event
	)
	err := s.d.UoW.Do(ctx, func(ctx context.Context, r ports.Repositories) error {
		var err error
		job, err = r.Jobs.Get(ctx, id)
		if err != nil {
			return err
		}
		changed, tasks, err := m(job)
		if err != nil || !changed {
			return err
		}
		if err := r.Jobs.Update(ctx, job); err != nil {
			return err
		}
		for _, t := range tasks {
			if err := r.Queue.Enqueue(ctx, t); err != nil {
				return fmt.Errorf("enfileirando %s: %w", t.Kind, err)
			}
		}
		events = job.PullEvents()
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.publish(ctx, events)
	return job, nil
}

func (s *Service) publish(ctx context.Context, events []domain.Event) {
	if len(events) > 0 && s.d.Events != nil {
		s.d.Events.Publish(ctx, events)
	}
}

func (s *Service) validate(ref domain.TemplateRef, raw []byte) (domain.Storyboard, error) {
	sb, err := domain.NewStoryboard(raw)
	if err != nil {
		return domain.Storyboard{}, err
	}
	issues, err := s.d.Catalog.Validate(ref, raw)
	if err != nil {
		return domain.Storyboard{}, fmt.Errorf("validando storyboard: %w", err)
	}
	if len(issues) > 0 {
		return domain.Storyboard{}, &ValidationError{Issues: issues}
	}
	return sb, nil
}
