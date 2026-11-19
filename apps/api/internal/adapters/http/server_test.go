package httpapi_test

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/adapters/events"
	httpapi "github.com/gahmorais/pipeline-video-gen/apps/api/internal/adapters/http"
	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/adapters/memory"
	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/adapters/schema"
	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/adapters/storage"
	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/application"
	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/domain"
	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/ports"
)

var _ httpapi.VideoService = (*application.Service)(nil)

const token = "segredo-de-teste"

var validStoryboard = mustRead("valid/promo-basic.json")

func mustRead(name string) []byte {
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "packages", "video-schema", "fixtures", name))
	if err != nil {
		panic(err)
	}
	return b
}

type fixtureGenerator struct{}

func (fixtureGenerator) Generate(context.Context, ports.GenerateInput) (ports.GenerateOutput, error) {
	return ports.GenerateOutput{Storyboard: validStoryboard, Model: "fixture"}, nil
}

type env struct {
	srv      *httptest.Server
	svc      *application.Service
	store    *memory.Store
	broker   *events.Broker
	mediaDir string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	catalog, err := schema.NewEmbeddedCatalog()
	if err != nil {
		t.Fatal(err)
	}
	e := &env{store: memory.NewStore(), broker: events.NewBroker(), mediaDir: t.TempDir()}
	e.svc = application.NewService(application.Deps{
		UoW: e.store, Generator: fixtureGenerator{}, Catalog: catalog, Events: e.broker,
		IDs: &seqIDs{}, Clock: clock{},
	})
	media, _ := storage.NewLocalFS(e.mediaDir)
	s, err := httpapi.NewServer(e.svc, e.broker, media, httpapi.Config{
		InternalToken: token, AllowedOrigin: "http://localhost:3000", SSEHeartbeat: 50 * time.Millisecond,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	e.srv = httptest.NewServer(s.Handler())
	t.Cleanup(e.srv.Close)
	return e
}

func (e *env) do(t *testing.T, method, path, body string, headers ...string) (*http.Response, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest(method, e.srv.URL+path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp, out
}

// readyJob cria um job via HTTP e gera o roteiro (simulando o storyboard worker).
func (e *env) readyJob(t *testing.T) string {
	t.Helper()
	resp, body := e.do(t, "POST", "/videos", `{"prompt":"ofertas da semana","templateId":"promo"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d %v", resp.StatusCode, body)
	}
	id := body["id"].(string)
	if err := e.svc.GenerateStoryboard(context.Background(), domain.JobID(id)); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestCreateAndGetVideo(t *testing.T) {
	e := newEnv(t)
	resp, body := e.do(t, "POST", "/videos", `{"prompt":"ofertas","templateId":"promo"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status %d: %v", resp.StatusCode, body)
	}
	id := body["id"].(string)
	if resp.Header.Get("Location") != "/videos/"+id {
		t.Fatalf("Location: %q", resp.Header.Get("Location"))
	}
	if body["status"] != "DRAFT" || body["storyboard"] != nil || body["render"] != nil {
		t.Fatalf("dto: %v", body)
	}
	if got := e.store.Tasks(); len(got) != 1 || got[0].Kind != ports.TaskGenerateStoryboard {
		t.Fatalf("fila: %+v", got)
	}

	resp, body = e.do(t, "GET", "/videos/"+id, "")
	if resp.StatusCode != http.StatusOK || body["prompt"] != "ofertas" {
		t.Fatalf("get: %d %v", resp.StatusCode, body)
	}
}

func TestErrorMapping(t *testing.T) {
	e := newEnv(t)
	id := e.readyJob(t)
	tests := []struct {
		name, method, path, body string
		headers                  []string
		want                     int
		wantType                 string
	}{
		{"job inexistente", "GET", "/videos/nope", "", nil, 404, "not-found"},
		{"id inseguro", "GET", "/videos/a%2Fb", "", nil, 400, "invalid-id"},
		{"prompt vazio", "POST", "/videos", `{"prompt":" ","templateId":"promo"}`, nil, 400, "invalid-prompt"},
		{"template desconhecido", "POST", "/videos", `{"prompt":"x","templateId":"nope"}`, nil, 400, "unknown-template"},
		{"campo desconhecido", "POST", "/videos", `{"prompt":"x","templateId":"promo","admin":true}`, nil, 400, "bad-request"},
		{"JSON malformado", "POST", "/videos", `{`, nil, 400, "bad-request"},
		{"retry fora de FAILED", "POST", "/videos/" + id + "/retry", "", nil, 409, "invalid-transition"},
		{"storyboard não-objeto", "PATCH", "/videos/" + id + "/storyboard", `[1]`, nil, 400, "invalid-storyboard"},
		{"idempotency key insegura", "POST", "/videos/" + id + "/render", "", []string{"Idempotency-Key", "../x"}, 400, "invalid-id"},
		{"mídia antes do render", "GET", "/videos/" + id + "/media", "", nil, 404, "not-found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, body := e.do(t, tt.method, tt.path, tt.body, tt.headers...)
			if resp.StatusCode != tt.want || body["type"] != tt.wantType {
				t.Fatalf("obtido %d %v, esperado %d %s", resp.StatusCode, body, tt.want, tt.wantType)
			}
			if ct := resp.Header.Get("Content-Type"); ct != "application/problem+json" {
				t.Fatalf("content-type: %s", ct)
			}
		})
	}
}

func TestUpdateStoryboardValidation(t *testing.T) {
	e := newEnv(t)
	id := e.readyJob(t)

	edited := strings.Replace(string(validStoryboard), "Ofertas da semana", "Black Friday", 1)
	resp, body := e.do(t, "PATCH", "/videos/"+id+"/storyboard", edited)
	if resp.StatusCode != http.StatusOK || !strings.Contains(mustJSON(body["storyboard"]), "Black Friday") {
		t.Fatalf("patch válido: %d %v", resp.StatusCode, body)
	}

	resp, body = e.do(t, "PATCH", "/videos/"+id+"/storyboard", `{"schemaVersion":1,"brand":{"primary":"azul","logoUrl":"a"},"scenes":[]}`)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("patch inválido: %d %v", resp.StatusCode, body)
	}
	issues := body["issues"].([]any)
	paths := map[string]bool{}
	for _, i := range issues {
		paths[i.(map[string]any)["path"].(string)] = true
	}
	if !paths["brand.primary"] || !paths["scenes"] {
		t.Fatalf("issues: %v", issues)
	}
}

func TestInternalEndpointsRequireToken(t *testing.T) {
	e := newEnv(t)
	id := e.readyJob(t)
	path := "/internal/videos/" + id + "/renders/r1/start"
	for _, auth := range []string{"", "Bearer errado", token} {
		resp, _ := e.do(t, "POST", path, "", "Authorization", auth)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("auth %q: esperado 401, obtido %d", auth, resp.StatusCode)
		}
	}
}

func TestFullRenderFlowAndMedia(t *testing.T) {
	e := newEnv(t)
	id := e.readyJob(t)
	auth := []string{"Authorization", "Bearer " + token}

	resp, body := e.do(t, "POST", "/videos/"+id+"/render", "", "Idempotency-Key", "rk-1")
	if resp.StatusCode != http.StatusAccepted || body["status"] != "RENDER_QUEUED" {
		t.Fatalf("render: %d %v", resp.StatusCode, body)
	}
	resp, _ = e.do(t, "POST", "/videos/"+id+"/render", "", "Idempotency-Key", "rk-1")
	if resp.StatusCode != http.StatusAccepted || len(e.store.Tasks()) != 2 {
		t.Fatalf("idempotência: %d tasks=%d", resp.StatusCode, len(e.store.Tasks()))
	}

	base := "/internal/videos/" + id + "/renders/rk-1"
	if resp, body := e.do(t, "POST", base+"/start", "", auth...); resp.StatusCode != 200 || body["status"] != "RENDERING" {
		t.Fatalf("start: %d %v", resp.StatusCode, body)
	}
	if resp, body := e.do(t, "POST", base+"/progress", `{"progress":0.5}`, auth...); resp.StatusCode != 200 ||
		body["render"].(map[string]any)["progress"] != 0.5 {
		t.Fatalf("progress: %d %v", resp.StatusCode, body)
	}
	if resp, body := e.do(t, "POST", "/internal/videos/"+id+"/renders/outro/progress", `{"progress":0.6}`, auth...); resp.StatusCode != 409 ||
		body["type"] != "render-mismatch" {
		t.Fatalf("worker obsoleto: %d %v", resp.StatusCode, body)
	}

	// complete exige que o arquivo exista
	key := id + "/rk-1.mp4"
	if resp, body := e.do(t, "POST", base+"/complete", `{"outputKey":"`+key+`"}`, auth...); resp.StatusCode != 404 {
		t.Fatalf("complete sem arquivo: %d %v", resp.StatusCode, body)
	}
	if resp, body := e.do(t, "POST", base+"/complete", `{"outputKey":"../../etc/passwd"}`, auth...); resp.StatusCode != 400 {
		t.Fatalf("complete com path traversal: %d %v", resp.StatusCode, body)
	}
	content := []byte("0123456789abcdef")
	_ = os.MkdirAll(filepath.Join(e.mediaDir, id), 0o755)
	_ = os.WriteFile(filepath.Join(e.mediaDir, key), content, 0o644)

	resp, body = e.do(t, "POST", base+"/complete", `{"outputKey":"`+key+`"}`, auth...)
	if resp.StatusCode != 200 || body["status"] != "COMPLETED" {
		t.Fatalf("complete: %d %v", resp.StatusCode, body)
	}
	mediaURL := body["mediaUrl"].(string)
	if mediaURL != "/videos/"+id+"/media?r=rk-1" {
		t.Fatalf("mediaUrl: %s", mediaURL)
	}

	// Range request (seek no player)
	req, _ := http.NewRequest("GET", e.srv.URL+mediaURL, nil)
	req.Header.Set("Range", "bytes=4-7")
	mresp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer mresp.Body.Close()
	got, _ := io.ReadAll(mresp.Body)
	if mresp.StatusCode != http.StatusPartialContent || string(got) != "4567" || mresp.Header.Get("Content-Type") != "video/mp4" {
		t.Fatalf("range: %d %q %s", mresp.StatusCode, got, mresp.Header.Get("Content-Type"))
	}
}

func TestFailAndRetryViaHTTP(t *testing.T) {
	e := newEnv(t)
	id := e.readyJob(t)
	auth := []string{"Authorization", "Bearer " + token}
	e.do(t, "POST", "/videos/"+id+"/render", "", "Idempotency-Key", "rk")
	e.do(t, "POST", "/internal/videos/"+id+"/renders/rk/start", "", auth...)

	resp, body := e.do(t, "POST", "/internal/videos/"+id+"/renders/rk/fail", `{"reason":"chrome crashed"}`, auth...)
	if resp.StatusCode != 200 || body["status"] != "FAILED" || body["failure"].(map[string]any)["reason"] != "chrome crashed" {
		t.Fatalf("fail: %d %v", resp.StatusCode, body)
	}
	resp, body = e.do(t, "POST", "/videos/"+id+"/retry", "")
	if resp.StatusCode != http.StatusAccepted || body["status"] != "RENDER_QUEUED" || body["failure"] != nil {
		t.Fatalf("retry: %d %v", resp.StatusCode, body)
	}
}

func TestCORS(t *testing.T) {
	e := newEnv(t)
	resp, _ := e.do(t, "OPTIONS", "/videos", "", "Origin", "http://localhost:3000")
	if resp.StatusCode != http.StatusNoContent || resp.Header.Get("Access-Control-Allow-Origin") != "http://localhost:3000" {
		t.Fatalf("preflight: %d %v", resp.StatusCode, resp.Header)
	}
	resp, _ = e.do(t, "GET", "/healthz", "", "Origin", "http://evil.example")
	if resp.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("origem não permitida não deve receber CORS")
	}
}

func TestSSEStreamsSnapshotsUntilClientCloses(t *testing.T) {
	e := newEnv(t)
	resp, body := e.do(t, "POST", "/videos", `{"prompt":"x","templateId":"promo"}`)
	if resp.StatusCode != 201 {
		t.Fatal(body)
	}
	id := body["id"].(string)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", e.srv.URL+"/videos/"+id+"/events", nil)
	stream, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Body.Close()
	if ct := stream.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content-type: %s", ct)
	}

	events := make(chan map[string]any, 10)
	sawPing := make(chan struct{}, 1)
	go func() {
		sc := bufio.NewScanner(stream.Body)
		for sc.Scan() {
			line := sc.Text()
			if line == ": ping" {
				select {
				case sawPing <- struct{}{}:
				default:
				}
			}
			if data, ok := strings.CutPrefix(line, "data: "); ok {
				var dto map[string]any
				_ = json.Unmarshal([]byte(data), &dto)
				events <- dto
			}
		}
		close(events)
	}()

	next := func() map[string]any {
		select {
		case ev, ok := <-events:
			if !ok {
				t.Fatal("stream encerrado")
			}
			return ev
		case <-ctx.Done():
			t.Fatal("timeout esperando evento")
		}
		return nil
	}

	if first := next(); first["status"] != "DRAFT" {
		t.Fatalf("snapshot inicial: %v", first)
	}

	// Mudanças publicadas pelo use case chegam como snapshots completos.
	go func() { _ = e.svc.GenerateStoryboard(context.Background(), domain.JobID(id)) }()
	seen := map[string]bool{}
	for !seen["STORYBOARD_READY"] {
		ev := next()
		seen[ev["status"].(string)] = true
		if ev["status"] == "STORYBOARD_READY" && ev["storyboard"] == nil {
			t.Fatal("snapshot sem storyboard")
		}
	}

	select {
	case <-sawPing:
	case <-ctx.Done():
		t.Fatal("heartbeat não recebido")
	}

	cancel()
	deadline := time.Now().Add(2 * time.Second)
	for e.broker.Subscribers(domain.JobID(id)) != 0 {
		if time.Now().After(deadline) {
			t.Fatal("assinatura SSE não foi liberada após o cliente desconectar")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

type seqIDs struct{ n int }

func (s *seqIDs) NewJobID() domain.JobID {
	s.n++
	return domain.JobID("job-" + string(rune('a'+s.n)))
}
func (s *seqIDs) NewRenderID() domain.RenderID {
	s.n++
	return domain.RenderID("render-" + string(rune('a'+s.n)))
}

type clock struct{}

func (clock) Now() time.Time { return time.Now().UTC() }
