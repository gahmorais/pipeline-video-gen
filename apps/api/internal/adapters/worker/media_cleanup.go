package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/adapters/storage"
)

// MediaFiles é o que a limpeza precisa do storage (interface no consumidor).
type MediaFiles interface {
	List(ctx context.Context) ([]storage.Entry, error)
	Remove(ctx context.Context, key string) error
}

// ReferenceSource informa quais arquivos são o vídeo atual de algum job.
type ReferenceSource interface {
	ReferencedOutputKeys(ctx context.Context) (map[string]struct{}, error)
}

// MediaCleanup remove do disco:
//   - parciais (.partial-*) mais antigos que PartialAfter (renders que morreram no meio);
//   - vídeos que não são mais o atual de nenhum job (substituídos por re-render ou
//     de jobs editados) mais antigos que OrphanAfter.
//
// O vídeo atual de cada job nunca é apagado. As idades mínimas evitam corrida com
// um render recém-concluído cujo resultado ainda não foi registrado na API.
type MediaCleanup struct {
	Files        MediaFiles
	References   ReferenceSource
	PartialAfter time.Duration
	OrphanAfter  time.Duration
	Logger       *slog.Logger
	Now          func() time.Time
}

type CleanupResult struct {
	Removed    int
	FreedBytes int64
}

func (c MediaCleanup) RunOnce(ctx context.Context) (CleanupResult, error) {
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	refs, err := c.References.ReferencedOutputKeys(ctx)
	if err != nil {
		return CleanupResult{}, err
	}
	entries, err := c.Files.List(ctx)
	if err != nil {
		return CleanupResult{}, err
	}

	var res CleanupResult
	for _, e := range entries {
		age := now().Sub(e.ModTime)
		_, referenced := refs[e.Key]
		remove := (e.Partial && age > c.PartialAfter) || (!e.Partial && !referenced && age > c.OrphanAfter)
		if !remove {
			continue
		}
		if err := c.Files.Remove(ctx, e.Key); err != nil {
			c.Logger.WarnContext(ctx, "falha ao remover mídia", "key", e.Key, "err", err)
			continue
		}
		res.Removed++
		res.FreedBytes += e.Size
	}
	return res, nil
}

// Run executa a limpeza periodicamente até o contexto ser cancelado.
func (c MediaCleanup) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		res, err := c.RunOnce(ctx)
		switch {
		case err != nil && ctx.Err() == nil:
			c.Logger.ErrorContext(ctx, "limpeza de mídia", "err", err)
		case res.Removed > 0:
			c.Logger.InfoContext(ctx, "mídia removida", "files", res.Removed, "freed_bytes", res.FreedBytes)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
