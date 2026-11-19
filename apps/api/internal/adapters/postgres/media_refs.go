package postgres

import (
	"context"
	"database/sql"
)

// MediaReferences informa quais arquivos de mídia ainda são o vídeo atual de algum job.
type MediaReferences struct {
	db *sql.DB
}

func NewMediaReferences(db *sql.DB) *MediaReferences { return &MediaReferences{db: db} }

func (m *MediaReferences) ReferencedOutputKeys(ctx context.Context) (map[string]struct{}, error) {
	rows, err := m.db.QueryContext(ctx, `SELECT output_key FROM video_jobs WHERE output_key <> ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := map[string]struct{}{}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		keys[k] = struct{}{}
	}
	return keys, rows.Err()
}
