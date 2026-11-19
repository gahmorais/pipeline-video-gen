package domain

import "time"

// Event é um evento de domínio emitido pelo agregado. Consumido pela camada de
// aplicação (ex.: publicar via SSE) após persistir o job.
type Event interface {
	JobID() JobID
	OccurredAt() time.Time
	EventName() string
}

type baseEvent struct {
	jobID JobID
	at    time.Time
}

func (e baseEvent) JobID() JobID          { return e.jobID }
func (e baseEvent) OccurredAt() time.Time { return e.at }

// StatusChanged é emitido em toda mudança de status.
type StatusChanged struct {
	baseEvent
	From   JobStatus
	To     JobStatus
	Reason string
}

func (StatusChanged) EventName() string { return "job.status_changed" }

// RenderProgressed é emitido quando o progresso do render avança.
type RenderProgressed struct {
	baseEvent
	RenderID RenderID
	Progress float64
}

func (RenderProgressed) EventName() string { return "job.render_progressed" }

// StoryboardUpdated é emitido quando o roteiro é gerado ou editado.
type StoryboardUpdated struct {
	baseEvent
}

func (StoryboardUpdated) EventName() string { return "job.storyboard_updated" }
