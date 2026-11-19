package domain

import (
	"encoding/json"
	"fmt"
	"time"
)

// Snapshot é a representação plana do agregado para persistência.
// Permite que adapters (ex.: Postgres) salvem/reidratem o job sem setters públicos.
type Snapshot struct {
	ID             JobID
	Prompt         Prompt
	Template       TemplateRef
	Status         JobStatus
	Storyboard     json.RawMessage
	RenderID       RenderID
	RenderProgress float64
	OutputKey      string
	Failure        *Failure
	RetryCount     int
	Version        int
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Snapshot exporta o estado atual.
func (j *VideoJob) Snapshot() Snapshot {
	return Snapshot{
		ID:             j.id,
		Prompt:         j.prompt,
		Template:       j.template,
		Status:         j.status,
		Storyboard:     j.storyboard.JSON(),
		RenderID:       j.renderID,
		RenderProgress: j.renderProgress,
		OutputKey:      j.outputKey,
		Failure:        j.Failure(),
		RetryCount:     j.retryCount,
		Version:        j.version,
		CreatedAt:      j.createdAt,
		UpdatedAt:      j.updatedAt,
	}
}

// Rehydrate reconstrói o agregado a partir do armazenamento, validando invariantes básicas.
// Não emite eventos.
func Rehydrate(s Snapshot) (*VideoJob, error) {
	if s.ID == "" {
		return nil, fmt.Errorf("%w: job id vazio", ErrInvalidID)
	}
	if !s.Status.IsValid() {
		return nil, fmt.Errorf("status desconhecido %q", s.Status)
	}
	if s.Status == StatusFailed && s.Failure == nil {
		return nil, fmt.Errorf("job %s em FAILED sem registro de falha", s.ID)
	}
	var sb Storyboard
	if len(s.Storyboard) > 0 {
		var err error
		if sb, err = NewStoryboard(s.Storyboard); err != nil {
			return nil, err
		}
	}
	var failure *Failure
	if s.Failure != nil {
		f := *s.Failure
		failure = &f
	}
	return &VideoJob{
		id:             s.ID,
		prompt:         s.Prompt,
		template:       s.Template,
		status:         s.Status,
		storyboard:     sb,
		renderID:       s.RenderID,
		renderProgress: s.RenderProgress,
		outputKey:      s.OutputKey,
		failure:        failure,
		retryCount:     s.RetryCount,
		version:        s.Version,
		createdAt:      s.CreatedAt,
		updatedAt:      s.UpdatedAt,
	}, nil
}

// SetVersion é usado exclusivamente pelo repositório após persistir (controle otimista).
func (j *VideoJob) SetVersion(v int) { j.version = v }
