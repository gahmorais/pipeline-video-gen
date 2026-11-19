package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/adapters/events"
	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/application"
	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/domain"
	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/ports"
)

// VideoService é o subconjunto de use cases usado pelo HTTP (interface no consumidor).
type VideoService interface {
	CreateVideo(ctx context.Context, in application.CreateVideoInput) (*domain.VideoJob, error)
	GetVideo(ctx context.Context, id domain.JobID) (*domain.VideoJob, error)
	UpdateStoryboard(ctx context.Context, id domain.JobID, raw []byte) (*domain.VideoJob, error)
	RequestRender(ctx context.Context, id domain.JobID, idempotencyKey string) (*domain.VideoJob, error)
	RetryJob(ctx context.Context, id domain.JobID) (*domain.VideoJob, error)
	StartRender(ctx context.Context, id domain.JobID, renderID domain.RenderID) (*domain.VideoJob, error)
	ReportRenderProgress(ctx context.Context, id domain.JobID, renderID domain.RenderID, progress float64) (*domain.VideoJob, error)
	CompleteRender(ctx context.Context, id domain.JobID, renderID domain.RenderID, outputKey string) (*domain.VideoJob, error)
	FailRender(ctx context.Context, id domain.JobID, renderID domain.RenderID, reason string) (*domain.VideoJob, error)
}

type Config struct {
	// InternalToken protege os endpoints usados pelos workers. Obrigatório.
	InternalToken string
	// AllowedOrigin habilita CORS para o frontend (ex.: http://localhost:3000). Vazio = sem CORS.
	AllowedOrigin string
	// MaxBodyBytes limita o corpo das requisições (storyboards são pequenos).
	MaxBodyBytes int64
	// SSEHeartbeat mantém conexões SSE vivas através de proxies.
	SSEHeartbeat time.Duration
}

type Server struct {
	svc    VideoService
	broker *events.Broker
	media  ports.MediaReader
	cfg    Config
	logger *slog.Logger
}

func NewServer(svc VideoService, broker *events.Broker, media ports.MediaReader, cfg Config, logger *slog.Logger) (*Server, error) {
	if cfg.InternalToken == "" {
		return nil, errors.New("InternalToken é obrigatório")
	}
	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = 1 << 20
	}
	if cfg.SSEHeartbeat <= 0 {
		cfg.SSEHeartbeat = 15 * time.Second
	}
	return &Server{svc: svc, broker: broker, media: media, cfg: cfg, logger: logger}, nil
}

// Handler monta as rotas (padrões de rota do net/http, Go >= 1.22).
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// Públicos (frontend)
	mux.HandleFunc("POST /videos", s.createVideo)
	mux.HandleFunc("GET /videos/{id}", s.getVideo)
	mux.HandleFunc("PATCH /videos/{id}/storyboard", s.updateStoryboard)
	mux.HandleFunc("POST /videos/{id}/render", s.requestRender)
	mux.HandleFunc("POST /videos/{id}/retry", s.retry)
	mux.HandleFunc("GET /videos/{id}/events", s.events)
	mux.HandleFunc("GET /videos/{id}/media", s.serveMedia)

	// Internos (workers): exigem token
	internal := http.NewServeMux()
	internal.HandleFunc("POST /internal/videos/{id}/renders/{renderId}/start", s.renderStart)
	internal.HandleFunc("POST /internal/videos/{id}/renders/{renderId}/progress", s.renderProgress)
	internal.HandleFunc("POST /internal/videos/{id}/renders/{renderId}/complete", s.renderComplete)
	internal.HandleFunc("POST /internal/videos/{id}/renders/{renderId}/fail", s.renderFail)
	mux.Handle("/internal/", s.requireToken(internal))

	return s.recoverer(s.logRequests(s.cors(mux)))
}

// --- Públicos ------------------------------------------------------------------

func (s *Server) createVideo(w http.ResponseWriter, r *http.Request) {
	var req createVideoRequest
	if err := s.decode(w, r, &req); err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	job, err := s.svc.CreateVideo(r.Context(), application.CreateVideoInput{Prompt: req.Prompt, TemplateID: req.TemplateID})
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	w.Header().Set("Location", "/videos/"+string(job.ID()))
	writeJSON(w, http.StatusCreated, toDTO(job))
}

func (s *Server) getVideo(w http.ResponseWriter, r *http.Request) {
	s.withJob(w, r, func(id domain.JobID) (*domain.VideoJob, error) {
		return s.svc.GetVideo(r.Context(), id)
	}, http.StatusOK)
}

func (s *Server) updateStoryboard(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, s.cfg.MaxBodyBytes))
	if err != nil {
		writeError(w, r, s.logger, fmt.Errorf("%w: corpo inválido ou grande demais", errBadRequest))
		return
	}
	s.withJob(w, r, func(id domain.JobID) (*domain.VideoJob, error) {
		return s.svc.UpdateStoryboard(r.Context(), id, body)
	}, http.StatusOK)
}

func (s *Server) requestRender(w http.ResponseWriter, r *http.Request) {
	key := r.Header.Get("Idempotency-Key")
	s.withJob(w, r, func(id domain.JobID) (*domain.VideoJob, error) {
		return s.svc.RequestRender(r.Context(), id, key)
	}, http.StatusAccepted)
}

func (s *Server) retry(w http.ResponseWriter, r *http.Request) {
	s.withJob(w, r, func(id domain.JobID) (*domain.VideoJob, error) {
		return s.svc.RetryJob(r.Context(), id)
	}, http.StatusAccepted)
}

func (s *Server) serveMedia(w http.ResponseWriter, r *http.Request) {
	id, err := domain.ParseJobID(r.PathValue("id"))
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	job, err := s.svc.GetVideo(r.Context(), id)
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	if job.Status() != domain.StatusCompleted || job.OutputKey() == "" {
		writeError(w, r, s.logger, fmt.Errorf("%w: vídeo ainda não renderizado", domain.ErrNotFound))
		return
	}
	f, err := s.media.Open(r.Context(), job.OutputKey())
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	// ServeContent trata Range, If-Modified-Since e HEAD (seek no player de vídeo).
	http.ServeContent(w, r, string(job.ID())+".mp4", f.ModTime(), f)
}

// --- Internos (workers) ------------------------------------------------------------

func (s *Server) renderStart(w http.ResponseWriter, r *http.Request) {
	s.withRender(w, r, func(id domain.JobID, rid domain.RenderID) (*domain.VideoJob, error) {
		return s.svc.StartRender(r.Context(), id, rid)
	})
}

func (s *Server) renderProgress(w http.ResponseWriter, r *http.Request) {
	var req progressRequest
	if err := s.decode(w, r, &req); err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	s.withRender(w, r, func(id domain.JobID, rid domain.RenderID) (*domain.VideoJob, error) {
		return s.svc.ReportRenderProgress(r.Context(), id, rid, req.Progress)
	})
}

func (s *Server) renderComplete(w http.ResponseWriter, r *http.Request) {
	var req completeRequest
	if err := s.decode(w, r, &req); err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	s.withRender(w, r, func(id domain.JobID, rid domain.RenderID) (*domain.VideoJob, error) {
		// Confirma que o arquivo existe antes de marcar COMPLETED.
		f, err := s.media.Open(r.Context(), req.OutputKey)
		if err != nil {
			return nil, err
		}
		_ = f.Close()
		return s.svc.CompleteRender(r.Context(), id, rid, req.OutputKey)
	})
}

func (s *Server) renderFail(w http.ResponseWriter, r *http.Request) {
	var req failRequest
	if err := s.decode(w, r, &req); err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	s.withRender(w, r, func(id domain.JobID, rid domain.RenderID) (*domain.VideoJob, error) {
		return s.svc.FailRender(r.Context(), id, rid, req.Reason)
	})
}

// --- Helpers -------------------------------------------------------------------

func (s *Server) withJob(w http.ResponseWriter, r *http.Request, fn func(domain.JobID) (*domain.VideoJob, error), status int) {
	id, err := domain.ParseJobID(r.PathValue("id"))
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	job, err := fn(id)
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	writeJSON(w, status, toDTO(job))
}

func (s *Server) withRender(w http.ResponseWriter, r *http.Request, fn func(domain.JobID, domain.RenderID) (*domain.VideoJob, error)) {
	rid, err := domain.ParseRenderID(r.PathValue("renderId"))
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	s.withJob(w, r, func(id domain.JobID) (*domain.VideoJob, error) { return fn(id, rid) }, http.StatusOK)
}

func (s *Server) decode(w http.ResponseWriter, r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, s.cfg.MaxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("%w: %v", errBadRequest, err)
	}
	return nil
}

// --- Middlewares ---------------------------------------------------------------

func (s *Server) requireToken(next http.Handler) http.Handler {
	expected := []byte("Bearer " + s.cfg.InternalToken)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := []byte(r.Header.Get("Authorization"))
		if subtle.ConstantTimeCompare(got, expected) != 1 {
			writeError(w, r, s.logger, errUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.AllowedOrigin != "" && r.Header.Get("Origin") == s.cfg.AllowedOrigin {
			h := w.Header()
			h.Set("Access-Control-Allow-Origin", s.cfg.AllowedOrigin)
			h.Set("Vary", "Origin")
			h.Set("Access-Control-Allow-Methods", "GET, POST, PATCH, OPTIONS")
			h.Set("Access-Control-Allow-Headers", "Content-Type, Idempotency-Key")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Flush repassa para o writer original (necessário para SSE).
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		attrs := []any{"method", r.Method, "path", r.URL.Path, "status", rec.status, "duration_ms", time.Since(start).Milliseconds()}
		if id := jobIDFromPath(r.URL.Path); id != "" {
			attrs = append(attrs, "job_id", id)
		}
		s.logger.InfoContext(r.Context(), "http request", attrs...)
	})
}

// jobIDFromPath extrai o id de /videos/{id}/... e /internal/videos/{id}/... para correlação nos logs.
// (O middleware roda fora do mux, então r.PathValue ainda não está disponível.)
func jobIDFromPath(path string) string {
	rest, ok := strings.CutPrefix(path, "/internal")
	if !ok {
		rest = path
	}
	rest, ok = strings.CutPrefix(rest, "/videos/")
	if !ok {
		return ""
	}
	id, _, _ := strings.Cut(rest, "/")
	if _, err := domain.ParseJobID(id); err != nil {
		return ""
	}
	return id
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if p := recover(); p != nil {
				if p == http.ErrAbortHandler {
					panic(p)
				}
				writeError(w, r, s.logger, fmt.Errorf("panic: %v", p))
			}
		}()
		next.ServeHTTP(w, r)
	})
}
