package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/domain"
)

// events transmite o estado do job via Server-Sent Events.
//
// Cada evento "job" carrega o DTO completo (snapshot), não um delta: o cliente
// só substitui o estado. Sinais coalescidos do broker + releitura do banco
// garantem que o cliente sempre converge para o estado atual.
// O servidor nunca encerra o stream por conta própria: o EventSource do browser
// reconectaria automaticamente, gerando um loop. Quem fecha é o cliente.
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	id, err := domain.ParseJobID(r.PathValue("id"))
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, r, s.logger, fmt.Errorf("streaming não suportado"))
		return
	}

	// Assina ANTES da leitura inicial: nenhuma mudança entre as duas se perde.
	signals, cancel := s.broker.Subscribe(id)
	defer cancel()

	job, err := s.svc.GetVideo(r.Context(), id)
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	lastVersion := -1
	send := func(j *domain.VideoJob) error {
		if j.Version() == lastVersion {
			return nil
		}
		lastVersion = j.Version()
		data, err := json.Marshal(toDTO(j))
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "id: %d\nevent: job\ndata: %s\n\n", j.Version(), data); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	if err := send(job); err != nil {
		return
	}

	heartbeat := time.NewTicker(s.cfg.SSEHeartbeat)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case <-signals:
			job, err := s.svc.GetVideo(r.Context(), id)
			if err != nil {
				s.logger.WarnContext(r.Context(), "sse: falha ao reler job", "job_id", id, "err", err)
				return
			}
			if err := send(job); err != nil {
				return
			}
		}
	}
}
