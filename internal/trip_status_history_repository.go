package app

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type StatusChange struct {
	TripID     uuid.UUID
	FromStatus *string
	ToStatus   string
	Reason     string
	ChangedAt  time.Time
}

type TripStatusHistoryRepository struct {
	pool         *pgxpool.Pool
	queryTimeout time.Duration
}

func NewTripStatusHistoryRepository(pool *pgxpool.Pool, queryTimeout time.Duration) *TripStatusHistoryRepository {
	return &TripStatusHistoryRepository{pool: pool, queryTimeout: queryTimeout}
}

func (r *TripStatusHistoryRepository) Insert(ctx context.Context, change StatusChange) error {
	ctx, cancel := withTimeout(ctx, r.queryTimeout)
	defer cancel()

	query, args, err := sb.Insert("trip_status_history").
		Columns("trip_id", "from_status", "to_status", "reason", "changed_at").
		Values(change.TripID, change.FromStatus, change.ToStatus, change.Reason, change.ChangedAt).
		ToSql()
	if err != nil {
		return fmt.Errorf("build insert trip status history: %w", err)
	}
	if _, err := executor(ctx, r.pool).Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("insert trip status history: %w", err)
	}
	return nil
}
