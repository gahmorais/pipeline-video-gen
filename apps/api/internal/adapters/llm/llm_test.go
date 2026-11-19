package llm_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/adapters/llm"
	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/adapters/schema"
	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/domain"
	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/ports"
)

var (
	discard = slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx     = context.Background()
)

func fixture(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "packages", "video-schema", "fixtures", "valid", "promo-basic.json"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func input(t *testing.T) ports.GenerateInput {
	t.Helper()
	cat, err := schema.NewEmbeddedCatalog()
	if err != nil {
		t.Fatal(err)
	}
	ref, _ := cat.Latest("promo")
	s, _ := cat.JSONSchema(ref)
	return ports.GenerateInput{JobID: "job-1", Prompt: "promo de scanners", Template: ref, JSONSchema: s}
}

// capture grava a requisição recebida e responde com status/corpo fixos.
type capture struct {
	path    string
	headers http.Header
	body    map[string]any
}

func fakeServer(t *testing.T, status int, response string) (*httptest.Server, *capture) {
	t.Helper()
	c := &capture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.path = r.URL.Path
		c.headers = r.Header.Clone()
		_ = json.NewDecoder(r.Body).Decode(&c.body)
		w.WriteHeader(status)
		_, _ = io.WriteString(w, response)
	}))
	t.Cleanup(srv.Close)
	return srv, c
}

func dig(m map[string]any, path ...any) any {
	var cur any = m
	for _, p := range path {
		switch k := p.(type) {
		case string:
			cur = cur.(map[string]any)[k]
		case int:
			cur = cur.([]any)[k]
		}
	}
	return cur
}

// --- Anthropic -------------------------------------------------------------------

func TestAnthropicRequestShapeAndToolUse(t *testing.T) {
	sb := fixture(t)
	resp := `{"model":"claude-opus-5-5","stop_reason":"tool_use",
		"content":[{"type":"text","text":"Aqui está."},{"type":"tool_use","name":"submit_storyboard","input":` + string(sb) + `}],
		"usage":{"input_tokens":100,"cache_read_input_tokens":1500,"cache_creation_input_tokens":0,"output_tokens":800}}`
	srv, c := fakeServer(t, 200, resp)

	g := llm.NewAnthropicGenerator(llm.AnthropicConfig{APIKey: "sk-test", Model: "claude-opus-5-5", BaseURL: srv.URL}, srv.Client())
	out, err := g.Generate(ctx, input(t))
	if err != nil {
		t.Fatal(err)
	}

	if c.path != "/v1/messages" || c.headers.Get("x-api-key") != "sk-test" || c.headers.Get("anthropic-version") != "2023-06-01" {
		t.Fatalf("requisição: %s %v", c.path, c.headers)
	}
	if dig(c.body, "model") != "claude-opus-5-5" || dig(c.body, "tool_choice", "type") != "auto" {
		t.Fatalf("corpo: %v", c.body)
	}
	tool := dig(c.body, "tools", 0).(map[string]any)
	if tool["name"] != "submit_storyboard" || dig(tool, "cache_control", "type") != "ephemeral" {
		t.Fatalf("tool: %v", tool)
	}
	schemaSent := tool["input_schema"].(map[string]any)
	if _, has := schemaSent["$schema"]; has || schemaSent["type"] != "object" {
		t.Fatalf("schema enviado deve omitir $schema: %v", schemaSent)
	}
	if dig(c.body, "system", 0, "cache_control", "type") != "ephemeral" {
		t.Fatal("system prompt deve ser marcado para cache")
	}
	if !strings.Contains(dig(c.body, "messages", 0, "content").(string), "promo de scanners") {
		t.Fatal("prompt do usuário ausente")
	}

	var got, want any
	_ = json.Unmarshal(out.Storyboard, &got)
	_ = json.Unmarshal(sb, &want)
	if !jsonEqual(got, want) {
		t.Fatalf("storyboard divergente: %s", out.Storyboard)
	}
	if out.Model != "claude-opus-5-5" || out.InputTokens != 1600 || out.OutputTokens != 800 {
		t.Fatalf("uso: %+v", out)
	}
}

func TestAnthropicFallsBackToTextJSON(t *testing.T) {
	resp := `{"content":[{"type":"text","text":"` + "```json\\n{\\\"a\\\": 1}\\n```" + `"}],"usage":{}}`
	srv, _ := fakeServer(t, 200, resp)
	g := llm.NewAnthropicGenerator(llm.AnthropicConfig{APIKey: "k", Model: "m", BaseURL: srv.URL}, srv.Client())
	out, err := g.Generate(ctx, input(t))
	if err != nil || string(out.Storyboard) != `{"a":1}` {
		t.Fatalf("out=%s err=%v", out.Storyboard, err)
	}
}

func TestAnthropicErrors(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr func(error) bool
	}{
		{"429 vira ProviderError", 429, `{"error":{"type":"rate_limit_error"}}`, func(err error) bool {
			var pe *llm.ProviderError
			return errors.As(err, &pe) && pe.StatusCode == 429 && pe.Provider == "anthropic"
		}},
		{"sem tool_use nem JSON", 200, `{"stop_reason":"end_turn","content":[{"type":"text","text":"não sei"}]}`, func(err error) bool {
			return errors.Is(err, llm.ErrNoStructuredOutput)
		}},
		{"corpo inválido", 200, `oops`, func(err error) bool { return err != nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, _ := fakeServer(t, tt.status, tt.body)
			g := llm.NewAnthropicGenerator(llm.AnthropicConfig{APIKey: "k", Model: "m", BaseURL: srv.URL}, srv.Client())
			if _, err := g.Generate(ctx, input(t)); !tt.wantErr(err) {
				t.Fatalf("erro inesperado: %v", err)
			}
		})
	}
}

// --- Gemini ---------------------------------------------------------------------

func TestGeminiRequestShapeAndParsing(t *testing.T) {
	sb := fixture(t)
	text, _ := json.Marshal(string(sb))
	resp := `{"candidates":[{"finishReason":"STOP","content":{"parts":[{"text":` + string(text) + `}]}}],
		"usageMetadata":{"promptTokenCount":1200,"candidatesTokenCount":300,"thoughtsTokenCount":200},
		"modelVersion":"gemini-3.5-flash"}`
	srv, c := fakeServer(t, 200, resp)

	g := llm.NewGeminiGenerator(llm.GeminiConfig{APIKey: "g-key", Model: "gemini-3.5-flash", BaseURL: srv.URL}, srv.Client())
	out, err := g.Generate(ctx, input(t))
	if err != nil {
		t.Fatal(err)
	}
	if c.path != "/v1beta/models/gemini-3.5-flash:generateContent" || c.headers.Get("x-goog-api-key") != "g-key" {
		t.Fatalf("requisição: %s %v", c.path, c.headers)
	}
	gc := dig(c.body, "generationConfig").(map[string]any)
	if gc["responseMimeType"] != "application/json" || dig(gc, "responseJsonSchema", "type") != "object" {
		t.Fatalf("generationConfig: %v", gc)
	}
	if !strings.Contains(dig(c.body, "systemInstruction", "parts", 0, "text").(string), "assets/logo.png") {
		t.Fatal("system instruction deve listar os assets permitidos")
	}
	if out.InputTokens != 1200 || out.OutputTokens != 500 || out.Model != "gemini-3.5-flash" {
		t.Fatalf("uso: %+v", out)
	}
	var got, want any
	_ = json.Unmarshal(out.Storyboard, &got)
	_ = json.Unmarshal(sb, &want)
	if !jsonEqual(got, want) {
		t.Fatal("storyboard divergente")
	}
}

func TestGeminiErrors(t *testing.T) {
	tests := []struct {
		name, body string
		status     int
	}{
		{"503", `{"error":{"code":503}}`, 503},
		{"sem candidatos", `{"candidates":[]}`, 200},
		{"texto não-JSON", `{"candidates":[{"finishReason":"SAFETY","content":{"parts":[{"text":"bloqueado"}]}}]}`, 200},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, _ := fakeServer(t, tt.status, tt.body)
			g := llm.NewGeminiGenerator(llm.GeminiConfig{APIKey: "k", Model: "m", BaseURL: srv.URL}, srv.Client())
			if _, err := g.Generate(ctx, input(t)); err == nil {
				t.Fatal("esperado erro")
			}
		})
	}
}

// --- Decorators -----------------------------------------------------------------

type stubGen struct {
	out   ports.GenerateOutput
	err   error
	calls int
}

func (s *stubGen) Generate(context.Context, ports.GenerateInput) (ports.GenerateOutput, error) {
	s.calls++
	return s.out, s.err
}

func TestFallbackGenerator(t *testing.T) {
	cat, _ := schema.NewEmbeddedCatalog()
	valid := ports.GenerateOutput{Storyboard: fixture(t), Model: "ok"}
	invalid := ports.GenerateOutput{Storyboard: []byte(`{"schemaVersion":1}`), Model: "ruim"}

	tests := []struct {
		name          string
		primary       stubGen
		secondary     stubGen
		wantModel     string
		wantErr       bool
		wantSecondary int
	}{
		{"primário válido", stubGen{out: valid}, stubGen{out: valid}, "ok", false, 0},
		{"primário com erro", stubGen{err: errors.New("503")}, stubGen{out: valid}, "ok", false, 1},
		{"primário fora do schema", stubGen{out: invalid}, stubGen{out: valid}, "ok", false, 1},
		{"ambos falham", stubGen{err: errors.New("503")}, stubGen{err: errors.New("529")}, "", true, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, s := tt.primary, tt.secondary
			out, err := llm.NewFallbackGenerator(&p, &s, cat, discard).Generate(ctx, input(t))
			if (err != nil) != tt.wantErr || out.Model != tt.wantModel || s.calls != tt.wantSecondary {
				t.Fatalf("out=%+v err=%v secondaryCalls=%d", out.Model, err, s.calls)
			}
			if tt.wantErr && (!strings.Contains(err.Error(), "503") || !strings.Contains(err.Error(), "529")) {
				t.Fatalf("erro deve conter as duas causas: %v", err)
			}
		})
	}
}

type recorder struct{ usages []ports.LLMUsage }

func (r *recorder) Record(_ context.Context, u ports.LLMUsage) error {
	r.usages = append(r.usages, u)
	return nil
}

func TestMeteredGeneratorRecordsUsageAndCost(t *testing.T) {
	rec := &recorder{}
	inner := &stubGen{out: ports.GenerateOutput{Storyboard: []byte(`{}`), Model: "claude-opus-5-5", InputTokens: 5000, OutputTokens: 5000}}
	m := llm.NewMeteredGenerator(inner, "fallback-name", llm.Price{InputPerMTok: 4, OutputPerMTok: 20}, rec, discard)
	if _, err := m.Generate(ctx, input(t)); err != nil {
		t.Fatal(err)
	}
	u := rec.usages[0]
	// 5000*4/1e6 + 5000*20/1e6 = 0.02 + 0.10
	if u.JobID != "job-1" || u.Model != "claude-opus-5-5" || !u.Success || abs(u.CostUSD-0.12) > 1e-9 {
		t.Fatalf("uso: %+v", u)
	}

	inner.err = errors.New("timeout")
	_, _ = m.Generate(ctx, input(t))
	if failed := rec.usages[1]; failed.Success || failed.Error != "timeout" {
		t.Fatalf("falha deve ser registrada: %+v", failed)
	}
}

func TestFixtureGeneratorReturnsCopy(t *testing.T) {
	f := llm.FixtureGenerator{Storyboard: []byte(`{"a":1}`)}
	out, _ := f.Generate(ctx, ports.GenerateInput{})
	out.Storyboard[0] = 'X'
	again, _ := f.Generate(ctx, ports.GenerateInput{})
	if string(again.Storyboard) != `{"a":1}` {
		t.Fatal("fixture compartilhou o buffer")
	}
}

func TestContextCancellationPropagates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Ler o corpo permite ao servidor detectar a desconexão do cliente.
		_, _ = io.Copy(io.Discard, r.Body)
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	defer srv.Close()
	g := llm.NewGeminiGenerator(llm.GeminiConfig{APIKey: "k", Model: "m", BaseURL: srv.URL}, srv.Client())
	c, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	if _, err := g.Generate(c, input(t)); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("esperado DeadlineExceeded, obtido %v", err)
	}
}

func jsonEqual(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

var _ = domain.JobID("")
