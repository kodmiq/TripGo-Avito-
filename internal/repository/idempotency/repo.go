package idempotency

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	domain "github.com/kodmiq/TripGo-Avito-/internal/domain/trip"
	"github.com/kodmiq/TripGo-Avito-/internal/infra/postgres"
	"github.com/kodmiq/TripGo-Avito-/internal/infra/postgres/transaction"
	"github.com/kodmiq/TripGo-Avito-/internal/model"
)

const idempotencyKeyConstraint = "idempotency_keys_pkey"

var sb = squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)

// Repository хранит ключи идемпотентности создания поездки.
type Repository struct {
	pool         *pgxpool.Pool
	queryTimeout time.Duration
}

func New(pool *pgxpool.Pool, queryTimeout time.Duration) *Repository {
	return &Repository{pool: pool, queryTimeout: queryTimeout}
}

func (r *Repository) Insert(ctx context.Context, rec model.IdempotencyRecord) error {
	ctx, cancel := postgres.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	query, args, err := sb.Insert("idempotency_keys").
		Columns("idempotency_key", "request_hash", "trip_id", "created_at", "expires_at").
		Values(rec.Key, rec.RequestHash, rec.TripID, rec.CreatedAt, rec.ExpiresAt).
		ToSql()
	if err != nil {
		return fmt.Errorf("build insert idempotency key: %w", err)
	}
	if _, err := transaction.Executor(ctx, r.pool).Exec(ctx, query, args...); err != nil {
		if isIdempotencyKeyConflict(err) {
			return domain.ErrIdempotencyKeyExists
		}
		return fmt.Errorf("insert idempotency key: %w", err)
	}
	return nil
}

func (r *Repository) Get(ctx context.Context, key uuid.UUID) (model.IdempotencyRecord, error) {
	ctx, cancel := postgres.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	query, args, err := sb.Select("idempotency_key", "request_hash", "trip_id", "created_at", "expires_at").
		From("idempotency_keys").
		Where(squirrel.Eq{"idempotency_key": key}).
		ToSql()
	if err != nil {
		return model.IdempotencyRecord{}, fmt.Errorf("build select idempotency key: %w", err)
	}

	var rec model.IdempotencyRecord
	err = transaction.Executor(ctx, r.pool).QueryRow(ctx, query, args...).Scan(
		&rec.Key,
		&rec.RequestHash,
		&rec.TripID,
		&rec.CreatedAt,
		&rec.ExpiresAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.IdempotencyRecord{}, domain.ErrIdempotencyKeyNotFound
	}
	if err != nil {
		return model.IdempotencyRecord{}, fmt.Errorf("select idempotency key: %w", err)
	}
	rec.CreatedAt = rec.CreatedAt.UTC()
	rec.ExpiresAt = rec.ExpiresAt.UTC()
	return rec, nil
}

// DeleteExact удаляет строку только с тем сроком, который увидел вызывающий.
// Чужую, уже обновлённую запись это не затрёт.
func (r *Repository) DeleteExact(ctx context.Context, key uuid.UUID, expiresAt time.Time) (bool, error) {
	ctx, cancel := postgres.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	query, args, err := sb.Delete("idempotency_keys").
		Where(squirrel.Eq{"idempotency_key": key, "expires_at": expiresAt}).
		ToSql()
	if err != nil {
		return false, fmt.Errorf("build delete idempotency key: %w", err)
	}
	tag, err := transaction.Executor(ctx, r.pool).Exec(ctx, query, args...)
	if err != nil {
		return false, fmt.Errorf("delete idempotency key: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

func isIdempotencyKeyConflict(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) &&
		pgErr.Code == "23505" &&
		pgErr.ConstraintName == idempotencyKeyConstraint
}
