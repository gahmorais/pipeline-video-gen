// Package application contém os use cases. Orquestra domínio e portas;
// não conhece HTTP, SQL nem provedores de LLM.
package application

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/ports"
)

// ErrUnknownTemplate indica template inexistente no catálogo.
var ErrUnknownTemplate = errors.New("template desconhecido")

// ValidationError indica storyboard fora do schema do template.
type ValidationError struct {
	Issues []ports.ValidationIssue
}

func (e *ValidationError) Error() string {
	parts := make([]string, 0, len(e.Issues))
	for _, i := range e.Issues {
		parts = append(parts, fmt.Sprintf("%s: %s", i.Path, i.Message))
	}
	return "storyboard não conforme ao schema: " + strings.Join(parts, "; ")
}
