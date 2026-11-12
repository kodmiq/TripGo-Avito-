package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TxManager открывает транзакцию так, чтобы бизнес-код не видел pgx.
// Репозиторий сам достаёт транзакцию из context.
type TxManager interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) error
}

type txKey struct{}

type pgTxManager struct {
	pool         *pgxpool.Pool
	queryTimeout time.Duration
}

func NewTxManager(pool *pgxpool.Pool, queryTimeout time.Duration) TxManager {
	return &pgTxManager{pool: pool, queryTimeout: queryTimeout}
}

// Do стартует транзакцию на уровне Read Committed.
// Вложенный вызов переиспользует уже открытую транзакцию и не делает COMMIT:
// фиксирует или откатывает только внешний Do.
// fn вернула ошибку или запаниковала — ROLLBACK, ошибка или паника идут наружу.
func (m *pgTxManager) Do(ctx context.Context, fn func(ctx context.Context) error) (err error) {
	if txFromContext(ctx) != nil {
		return fn(ctx)
	}

	beginCtx, cancelBegin := withTimeout(ctx, m.queryTimeout)
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

	commitCtx, cancelCommit := withTimeout(context.WithoutCancel(ctx), m.queryTimeout)
	defer cancelCommit()
	if err = tx.Commit(commitCtx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

func (m *pgTxManager) rollback(ctx context.Context, tx pgx.Tx) {
	rbCtx, cancel := withTimeout(context.WithoutCancel(ctx), m.queryTimeout)
	defer cancel()
	if rbErr := tx.Rollback(rbCtx); rbErr != nil {
		slog.Error("rollback transaction", "err", rbErr)
	}
}

func txFromContext(ctx context.Context) pgx.Tx {
	tx, _ := ctx.Value(txKey{}).(pgx.Tx)
	return tx
}

// executor возвращает транзакцию из context, а если Do не вызывался — пул.
func executor(ctx context.Context, pool *pgxpool.Pool) querier {
	if tx := txFromContext(ctx); tx != nil {
		return tx
	}
	return pool
}
