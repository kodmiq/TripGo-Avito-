package position

import (
	"context"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kodmiq/TripGo-Avito-/internal/infra/postgres/transaction"
)

var sb = squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)

// Repository хранит точки маршрута. Транзакцию берёт из context, если её открыл Manager.
type Repository struct {
	pool         *pgxpool.Pool
	queryTimeout time.Duration
}

func New(pool *pgxpool.Pool, queryTimeout time.Duration) *Repository {
	return &Repository{pool: pool, queryTimeout: queryTimeout}
}

func (r *Repository) query(ctx context.Context) transaction.Querier {
	return transaction.Executor(ctx, r.pool)
}
