// Package httpapi expõe os use cases via REST + SSE (net/http da stdlib).
package httpapi

import (
	"encoding/json"
	"net/url"
	"time"

	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/domain"
)

// VideoDTO é a representação pública do job. Mantida separada do domínio
// para que mudanças internas não quebrem o contrato com o frontend.
type VideoDTO struct {
	ID         string          `json:"id"`
	Status     string          `json:"status"`
	Prompt     string          `json:"prompt"`
	Template   TemplateDTO     `json:"template"`
	Storyboard json.RawMessage `json:"storyboard"`
	Render     *RenderDTO      `json:"render"`
	MediaURL   string          `json:"mediaUrl,omitempty"`
	Failure    *FailureDTO     `json:"failure"`
	RetryCount int             `json:"retryCount"`
	Version    int             `json:"version"`
	CreatedAt  time.Time       `json:"createdAt"`
	UpdatedAt  time.Time       `json:"updatedAt"`
}

type TemplateDTO struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
}

type RenderDTO struct {
	ID       string  `json:"id"`
	Progress float64 `json:"progress"`
}

type FailureDTO struct {
	Reason string    `json:"reason"`
	From   string    `json:"from"`
	At     time.Time `json:"at"`
}

func toDTO(j *domain.VideoJob) VideoDTO {
	s := j.Snapshot()
	dto := VideoDTO{
		ID:         string(s.ID),
		Status:     string(s.Status),
		Prompt:     string(s.Prompt),
		Template:   TemplateDTO{ID: s.Template.ID, Version: s.Template.Version},
		Storyboard: s.Storyboard,
		RetryCount: s.RetryCount,
		Version:    s.Version,
		CreatedAt:  s.CreatedAt,
		UpdatedAt:  s.UpdatedAt,
	}
	if len(dto.Storyboard) == 0 {
		dto.Storyboard = json.RawMessage("null")
	}
	if s.RenderID != "" {
		dto.Render = &RenderDTO{ID: string(s.RenderID), Progress: s.RenderProgress}
	}
	if s.Status == domain.StatusCompleted && s.OutputKey != "" {
		// O renderId na URL invalida caches quando um novo render substitui o anterior.
		dto.MediaURL = "/videos/" + url.PathEscape(string(s.ID)) + "/media?r=" + url.QueryEscape(string(s.RenderID))
	}
	if s.Failure != nil {
		dto.Failure = &FailureDTO{Reason: s.Failure.Reason, From: string(s.Failure.From), At: s.Failure.At}
	}
	return dto
}

type createVideoRequest struct {
	Prompt     string `json:"prompt"`
	TemplateID string `json:"templateId"`
}

type progressRequest struct {
	Progress float64 `json:"progress"`
}

type completeRequest struct {
	OutputKey string `json:"outputKey"`
}

type failRequest struct {
	Reason string `json:"reason"`
}
