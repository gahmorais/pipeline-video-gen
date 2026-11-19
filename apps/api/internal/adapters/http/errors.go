package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/adapters/storage"
	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/application"
	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/domain"
	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/ports"
)

// problem segue o formato RFC 9457 (application/problem+json).
type problem struct {
	Type   string                  `json:"type"`
	Title  string                  `json:"title"`
	Status int                     `json:"status"`
	Detail string                  `json:"detail,omitempty"`
	Issues []ports.ValidationIssue `json:"issues,omitempty"`
}

// errorMapping traduz erros de domínio/aplicação em respostas HTTP.
// A ordem importa: o primeiro que casar vence.
var errorMapping = []struct {
	target error
	status int
	code   string
}{
	{domain.ErrNotFound, http.StatusNotFound, "not-found"},
	{domain.ErrInvalidTransition, http.StatusConflict, "invalid-transition"},
	{domain.ErrRenderMismatch, http.StatusConflict, "render-mismatch"},
	{domain.ErrConcurrentUpdate, http.StatusConflict, "concurrent-update"},
	{domain.ErrRetryLimit, http.StatusConflict, "retry-limit"},
	{domain.ErrInvalidPrompt, http.StatusBadRequest, "invalid-prompt"},
	{domain.ErrInvalidTemplate, http.StatusBadRequest, "invalid-template"},
	{application.ErrUnknownTemplate, http.StatusBadRequest, "unknown-template"},
	{domain.ErrInvalidStoryboard, http.StatusBadRequest, "invalid-storyboard"},
	{domain.ErrInvalidProgress, http.StatusBadRequest, "invalid-progress"},
	{domain.ErrInvalidID, http.StatusBadRequest, "invalid-id"},
	{storage.ErrInvalidKey, http.StatusBadRequest, "invalid-media-key"},
	{errBadRequest, http.StatusBadRequest, "bad-request"},
	{errUnauthorized, http.StatusUnauthorized, "unauthorized"},
}

var (
	errBadRequest   = errors.New("requisição inválida")
	errUnauthorized = errors.New("não autorizado")
)

func writeError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	var verr *application.ValidationError
	if errors.As(err, &verr) {
		writeProblem(w, problem{
			Type: "validation-error", Title: "Storyboard fora do schema do template",
			Status: http.StatusUnprocessableEntity, Issues: verr.Issues,
		})
		return
	}
	for _, m := range errorMapping {
		if errors.Is(err, m.target) {
			writeProblem(w, problem{Type: m.code, Title: http.StatusText(m.status), Status: m.status, Detail: err.Error()})
			return
		}
	}
	// Erros inesperados: detalhes só no log, nunca na resposta.
	logger.ErrorContext(r.Context(), "erro interno", "err", err, "method", r.Method, "path", r.URL.Path)
	writeProblem(w, problem{Type: "internal", Title: "Erro interno", Status: http.StatusInternalServerError})
}

func writeProblem(w http.ResponseWriter, p problem) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(p.Status)
	_ = json.NewEncoder(w).Encode(p)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
