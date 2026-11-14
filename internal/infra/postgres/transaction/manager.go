package transaction

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Querier — то, чем репозиторий выполняет запрос: транзакция из context или пул.
type Querier interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type txKey struct{}

// Manager открывает транзакцию так, чтобы usecase не видел pgx.
type Manager struct {
	pool         *pgxpool.Pool
	queryTimeout time.Duration
	log          *slog.Logger
}

func New(pool *pgxpool.Pool, queryTimeout time.Duration, log *slog.Logger) *Manager {
	return &Manager{pool: pool, queryTimeout: queryTimeout, log: log}
}

// Do стартует транзакцию на уровне Read Committed.
// Вложенный вызов переиспользует уже открытую транзакцию и не делает COMMIT:
// фиксирует или откатывает только внешний Do.
// fn вернула ошибку или запаниковала — ROLLBACK, ошибка или паника идут наружу.
func (m *Manager) Do(ctx context.Context, fn func(ctx context.Context) error) (err error) {
	if txFromContext(ctx) != nil {
		return fn(ctx)
	}

	beginCtx, cancelBegin := context.WithTimeout(ctx, m.queryTimeout)
	tx, err := m.pool.BeginTx(beginCtx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	cancelBegin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			m.rollback(ctx, tx)
			panic(p)
		}
	}()

	txCtx := context.WithValue(ctx, txKey{}, tx)
	if err = fn(txCtx); err != nil {
		m.rollback(ctx, tx)
		return err
	}

	commitCtx, cancelCommit := context.WithTimeout(context.WithoutCancel(ctx), m.queryTimeout)
	defer cancelCommit()
	if err = tx.Commit(commitCtx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

func (m *Manager) rollback(ctx context.Context, tx pgx.Tx) {
	rbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), m.queryTimeout)
	defer cancel()
	if rbErr := tx.Rollback(rbCtx); rbErr != nil {
		m.log.ErrorContext(ctx, "rollback transaction", "err", rbErr)
	}
}

// Executor возвращает транзакцию из context, а если Do не вызывался — пул.
func Executor(ctx context.Context, pool *pgxpool.Pool) Querier {
	if tx := txFromContext(ctx); tx != nil {
		return tx
	}
	return pool
}

func txFromContext(ctx context.Context) pgx.Tx {
	tx, _ := ctx.Value(txKey{}).(pgx.Tx)
	return tx
}
