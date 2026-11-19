package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/adapters/llm"
	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/ports"
)

// Defaults de modelo e preço (USD por milhão de tokens). Preços mudam:
// confira as tabelas oficiais e sobrescreva via variáveis de ambiente.
const (
	defaultGeminiModel    = "gemini-3.5-flash"
	defaultAnthropicModel = "claude-opus-5-5"
)

var (
	defaultGeminiPrice    = llm.Price{InputPerMTok: 1.50, OutputPerMTok: 9.00}
	defaultAnthropicPrice = llm.Price{InputPerMTok: 4.00, OutputPerMTok: 20.00}
)

type llmConfig struct {
	Provider        string // auto | fallback | gemini | anthropic | fixture
	GeminiKey       string
	GeminiModel     string
	GeminiPrice     llm.Price
	AnthropicKey    string
	AnthropicModel  string
	AnthropicPrice  llm.Price
	RequestTimeout  time.Duration
	FixtureFilePath string
}

func loadLLMConfig(getenv func(string) string) (llmConfig, error) {
	or := func(k, d string) string {
		if v := getenv(k); v != "" {
			return v
		}
		return d
	}
	price := func(prefix string, d llm.Price) (llm.Price, error) {
		p := d
		for _, f := range []struct {
			key string
			dst *float64
		}{{prefix + "_PRICE_INPUT", &p.InputPerMTok}, {prefix + "_PRICE_OUTPUT", &p.OutputPerMTok}} {
			if raw := getenv(f.key); raw != "" {
				v, err := strconv.ParseFloat(raw, 64)
				if err != nil || v < 0 {
					return p, fmt.Errorf("%s inválido: %q", f.key, raw)
				}
				*f.dst = v
			}
		}
		return p, nil
	}
	cfg := llmConfig{
		Provider:        or("STORYBOARD_PROVIDER", "auto"),
		GeminiKey:       getenv("GEMINI_API_KEY"),
		GeminiModel:     or("GEMINI_MODEL", defaultGeminiModel),
		AnthropicKey:    getenv("ANTHROPIC_API_KEY"),
		AnthropicModel:  or("ANTHROPIC_MODEL", defaultAnthropicModel),
		RequestTimeout:  2 * time.Minute,
		FixtureFilePath: getenv("STORYBOARD_FIXTURE"),
	}
	var err error
	if cfg.GeminiPrice, err = price("GEMINI", defaultGeminiPrice); err != nil {
		return cfg, err
	}
	if cfg.AnthropicPrice, err = price("ANTHROPIC", defaultAnthropicPrice); err != nil {
		return cfg, err
	}
	if cfg.Provider == "auto" {
		switch {
		case cfg.GeminiKey != "" && cfg.AnthropicKey != "":
			cfg.Provider = "fallback"
		case cfg.GeminiKey != "":
			cfg.Provider = "gemini"
		case cfg.AnthropicKey != "":
			cfg.Provider = "anthropic"
		default:
			return cfg, fmt.Errorf("nenhuma chave de LLM configurada: defina GEMINI_API_KEY e/ou ANTHROPIC_API_KEY, ou STORYBOARD_PROVIDER=fixture")
		}
	}
	return cfg, nil
}

// buildGenerator monta: Metered(provedor) para cada provedor e, com os dois, Fallback(Gemini → Opus).
func buildGenerator(cfg llmConfig, catalog ports.TemplateCatalog, recorder ports.LLMUsageRecorder, logger *slog.Logger) (ports.StoryboardGenerator, error) {
	client := &http.Client{Timeout: cfg.RequestTimeout}
	gemini := func() (ports.StoryboardGenerator, error) {
		if cfg.GeminiKey == "" {
			return nil, fmt.Errorf("GEMINI_API_KEY ausente")
		}
		g := llm.NewGeminiGenerator(llm.GeminiConfig{APIKey: cfg.GeminiKey, Model: cfg.GeminiModel}, client)
		return llm.NewMeteredGenerator(g, cfg.GeminiModel, cfg.GeminiPrice, recorder, logger), nil
	}
	anthropic := func() (ports.StoryboardGenerator, error) {
		if cfg.AnthropicKey == "" {
			return nil, fmt.Errorf("ANTHROPIC_API_KEY ausente")
		}
		a := llm.NewAnthropicGenerator(llm.AnthropicConfig{APIKey: cfg.AnthropicKey, Model: cfg.AnthropicModel}, client)
		return llm.NewMeteredGenerator(a, cfg.AnthropicModel, cfg.AnthropicPrice, recorder, logger), nil
	}

	switch cfg.Provider {
	case "gemini":
		return gemini()
	case "anthropic":
		return anthropic()
	case "fallback":
		primary, err := gemini()
		if err != nil {
			return nil, err
		}
		secondary, err := anthropic()
		if err != nil {
			return nil, err
		}
		return llm.NewFallbackGenerator(primary, secondary, catalog, logger), nil
	case "fixture":
		if cfg.FixtureFilePath == "" {
			return nil, fmt.Errorf("STORYBOARD_FIXTURE deve apontar para um storyboard JSON quando STORYBOARD_PROVIDER=fixture")
		}
		data, err := os.ReadFile(cfg.FixtureFilePath)
		if err != nil {
			return nil, err
		}
		return llm.FixtureGenerator{Storyboard: data}, nil
	default:
		return nil, fmt.Errorf("STORYBOARD_PROVIDER desconhecido: %q", cfg.Provider)
	}
}
