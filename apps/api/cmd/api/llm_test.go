package main

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/adapters/llm"
	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/adapters/schema"
)

func envMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadLLMConfigProviderSelection(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		want    string
		wantErr bool
	}{
		{"duas chaves → fallback", map[string]string{"GEMINI_API_KEY": "g", "ANTHROPIC_API_KEY": "a"}, "fallback", false},
		{"só gemini", map[string]string{"GEMINI_API_KEY": "g"}, "gemini", false},
		{"só anthropic", map[string]string{"ANTHROPIC_API_KEY": "a"}, "anthropic", false},
		{"nenhuma chave", map[string]string{}, "", true},
		{"fixture explícito", map[string]string{"STORYBOARD_PROVIDER": "fixture"}, "fixture", false},
		{"preço inválido", map[string]string{"GEMINI_API_KEY": "g", "GEMINI_PRICE_INPUT": "abc"}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := loadLLMConfig(envMap(tt.env))
			if (err != nil) != tt.wantErr {
				t.Fatalf("err=%v", err)
			}
			if !tt.wantErr && cfg.Provider != tt.want {
				t.Fatalf("provider=%s", cfg.Provider)
			}
		})
	}
}

func TestLoadLLMConfigDefaultsAndOverrides(t *testing.T) {
	cfg, err := loadLLMConfig(envMap(map[string]string{
		"ANTHROPIC_API_KEY": "a", "ANTHROPIC_PRICE_OUTPUT": "25", "ANTHROPIC_MODEL": "claude-x",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AnthropicModel != "claude-x" || cfg.AnthropicPrice != (llm.Price{InputPerMTok: 4, OutputPerMTok: 25}) {
		t.Fatalf("cfg: %+v", cfg)
	}
	if cfg.GeminiModel != defaultGeminiModel {
		t.Fatalf("modelo padrão: %s", cfg.GeminiModel)
	}
}

func TestBuildGenerator(t *testing.T) {
	cat, _ := schema.NewEmbeddedCatalog()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	fixturePath := filepath.Join("..", "..", "..", "..", "packages", "video-schema", "fixtures", "valid", "promo-basic.json")
	if _, err := os.Stat(fixturePath); err != nil {
		t.Fatal(err)
	}

	ok := []llmConfig{
		{Provider: "gemini", GeminiKey: "g", GeminiModel: "m"},
		{Provider: "anthropic", AnthropicKey: "a", AnthropicModel: "m"},
		{Provider: "fallback", GeminiKey: "g", AnthropicKey: "a"},
		{Provider: "fixture", FixtureFilePath: fixturePath},
	}
	for _, cfg := range ok {
		if _, err := buildGenerator(cfg, cat, nil, logger); err != nil {
			t.Errorf("%s: %v", cfg.Provider, err)
		}
	}
	bad := []llmConfig{
		{Provider: "gemini"},
		{Provider: "fallback", GeminiKey: "g"},
		{Provider: "fixture"},
		{Provider: "openai"},
	}
	for _, cfg := range bad {
		if _, err := buildGenerator(cfg, cat, nil, logger); err == nil {
			t.Errorf("%s deveria falhar", cfg.Provider)
		}
	}
}
