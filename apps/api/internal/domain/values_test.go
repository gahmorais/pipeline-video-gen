package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/domain"
)

func TestNewPrompt(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    domain.Prompt
		wantErr error
	}{
		{"normal", "vídeo de ofertas", "vídeo de ofertas", nil},
		{"remove espaços", "  oi  ", "oi", nil},
		{"vazio", "   ", "", domain.ErrInvalidPrompt},
		{"no limite (runes, não bytes)", strings.Repeat("ç", domain.MaxPromptChars), domain.Prompt(strings.Repeat("ç", domain.MaxPromptChars)), nil},
		{"acima do limite", strings.Repeat("a", domain.MaxPromptChars+1), "", domain.ErrInvalidPrompt},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := domain.NewPrompt(tt.in)
			if !errors.Is(err, tt.wantErr) || got != tt.want {
				t.Fatalf("NewPrompt(%q) = %q, %v", tt.in[:min(len(tt.in), 20)], got, err)
			}
		})
	}
}

func TestParseIDs(t *testing.T) {
	valid := []string{"abc", "job_1", "A-b-9", strings.Repeat("x", 64)}
	invalid := []string{"", "../x", "a/b", "a b", "é", strings.Repeat("x", 65)}
	for _, s := range valid {
		if _, err := domain.ParseJobID(s); err != nil {
			t.Errorf("ParseJobID(%q) deveria ser válido: %v", s, err)
		}
		if _, err := domain.ParseRenderID(s); err != nil {
			t.Errorf("ParseRenderID(%q) deveria ser válido: %v", s, err)
		}
	}
	for _, s := range invalid {
		if _, err := domain.ParseJobID(s); !errors.Is(err, domain.ErrInvalidID) {
			t.Errorf("ParseJobID(%q) deveria falhar", s)
		}
	}
}

func TestNewTemplateRef(t *testing.T) {
	if ref, err := domain.NewTemplateRef("promo", 1); err != nil || ref.String() != "promo@1" {
		t.Fatalf("válido: %v %v", ref, err)
	}
	for _, tc := range []struct {
		id string
		v  int
	}{{"", 1}, {"promo", 0}, {"pro mo", 1}} {
		if _, err := domain.NewTemplateRef(tc.id, tc.v); !errors.Is(err, domain.ErrInvalidTemplate) {
			t.Errorf("NewTemplateRef(%q,%d) deveria falhar", tc.id, tc.v)
		}
	}
}

func TestNewStoryboard(t *testing.T) {
	valid := []string{`{}`, ` {"a":1} `}
	invalid := []string{``, `[]`, `"x"`, `{`, `null`}
	for _, s := range valid {
		if _, err := domain.NewStoryboard([]byte(s)); err != nil {
			t.Errorf("%q deveria ser válido: %v", s, err)
		}
	}
	for _, s := range invalid {
		if _, err := domain.NewStoryboard([]byte(s)); !errors.Is(err, domain.ErrInvalidStoryboard) {
			t.Errorf("%q deveria falhar", s)
		}
	}
}

func TestNewVideoJobValidation(t *testing.T) {
	tpl, _ := domain.NewTemplateRef("promo", 1)
	if _, err := domain.NewVideoJob("", "p", tpl, t0); !errors.Is(err, domain.ErrInvalidID) {
		t.Error("id vazio deveria falhar")
	}
	if _, err := domain.NewVideoJob("j", "", tpl, t0); !errors.Is(err, domain.ErrInvalidPrompt) {
		t.Error("prompt vazio deveria falhar")
	}
	if _, err := domain.NewVideoJob("j", "p", domain.TemplateRef{}, t0); !errors.Is(err, domain.ErrInvalidTemplate) {
		t.Error("template vazio deveria falhar")
	}
}

func TestStatusIsValid(t *testing.T) {
	for _, s := range domain.AllStatuses {
		if !s.IsValid() {
			t.Errorf("%s deveria ser válido", s)
		}
	}
	if domain.JobStatus("X").IsValid() {
		t.Error("status desconhecido não deveria ser válido")
	}
}
