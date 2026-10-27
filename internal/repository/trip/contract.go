package trip

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kodmiq/TripGo-Avito-/internal/infra/postgres/transaction"
)

const activeDriverConstraint = "trips_one_active_driver_idx"

const tripReturning = "id, user_id, driver_id, start_latitude, start_longitude, end_latitude, end_longitude, price, status, started_at, finished_at"

// row — минимальный контракт результата QueryRow. pgx.Row ему удовлетворяет.
type row interface {
	Scan(dest ...any) error
}

// Repository хранит поездки. Транзакцию берёт из context, если её открыл Manager.
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

// HistoryRepository пишет журнал переходов. Отдельный тип, чтобы usecase зависел
// только от того метода, который вызывает.
type HistoryRepository struct {
	pool         *pgxpool.Pool
	queryTimeout time.Duration
}

func NewHistory(pool *pgxpool.Pool, queryTimeout time.Duration) *HistoryRepository {
	return &HistoryRepository{pool: pool, queryTimeout: queryTimeout}
}

func (r *HistoryRepository) query(ctx context.Context) transaction.Querier {
	return transaction.Executor(ctx, r.pool)
}
