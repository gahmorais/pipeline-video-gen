package domain

import (
	"errors"
	"fmt"
)

// Erros sentinela do domínio. Use errors.Is para testá-los.
var (
	ErrInvalidTransition = errors.New("transição de estado inválida")
	ErrRenderMismatch    = errors.New("renderId não corresponde ao render atual do job")
	ErrInvalidPrompt     = errors.New("prompt inválido")
	ErrInvalidTemplate   = errors.New("template inválido")
	ErrInvalidStoryboard = errors.New("storyboard inválido")
	ErrInvalidProgress   = errors.New("progresso inválido")
	ErrInvalidID         = errors.New("identificador inválido")
	ErrRetryLimit        = errors.New("limite de tentativas excedido")
	ErrNotFound          = errors.New("job não encontrado")
	ErrConcurrentUpdate  = errors.New("job modificado concorrentemente")
)

// TransitionError detalha uma transição rejeitada. errors.Is(err, ErrInvalidTransition) == true.
type TransitionError struct {
	From   JobStatus
	Action string
}

func (e *TransitionError) Error() string {
	return fmt.Sprintf("%s: ação %q não permitida no estado %s", ErrInvalidTransition, e.Action, e.From)
}

func (e *TransitionError) Unwrap() error { return ErrInvalidTransition }
