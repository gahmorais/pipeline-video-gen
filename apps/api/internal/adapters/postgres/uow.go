package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/gahmorais/pipeline-video-gen/apps/api/internal/ports"
)

// UnitOfWork executa a função em uma transação, entregando repositórios
// ligados a ela (job + fila gravados atomicamente).
type UnitOfWork struct {
	db *sql.DB
}

var _ ports.UnitOfWork = (*UnitOfWork)(nil)

func NewUnitOfWork(db *sql.DB) *UnitOfWork { return &UnitOfWork{db: db} }

func (u *UnitOfWork) Do(ctx context.Context, fn func(context.Context, ports.Repositories) error) (err error) {
	tx, err := u.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("iniciando transação: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
				err = errors.Join(err, fmt.Errorf("rollback: %w", rbErr))
			}
		}
	}()

	if err = fn(ctx, ports.Repositories{
		Jobs:  &jobRepository{q: tx},
		Queue: &taskProducer{q: tx},
	}); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}
