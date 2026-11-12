package app

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
)

const idempotencyKeyConstraint = "idempotency_keys_pkey"

var ErrIdempotencyKeyExists = errors.New("idempotency key exists")

type IdempotencyRecord struct {
	Key         uuid.UUID
	RequestHash string
	TripID      uuid.UUID
	CreatedAt   time.Time
	ExpiresAt   time.Time
}

type IdempotencyRepository struct {
	pool         *pgxpool.Pool
	queryTimeout time.Duration
}

func NewIdempotencyRepository(pool *pgxpool.Pool, queryTimeout time.Duration) *IdempotencyRepository {
	return &IdempotencyRepository{pool: pool, queryTimeout: queryTimeout}
}

func (r *IdempotencyRepository) Insert(ctx context.Context, rec IdempotencyRecord) error {
	ctx, cancel := withTimeout(ctx, r.queryTimeout)
	defer cancel()

	query, args, err := sb.Insert("idempotency_keys").
		Columns("idempotency_key", "request_hash", "trip_id", "created_at", "expires_at").
		Values(rec.Key, rec.RequestHash, rec.TripID, rec.CreatedAt, rec.ExpiresAt).
		ToSql()
	if err != nil {
		return fmt.Errorf("build insert idempotency key: %w", err)
	}
	if _, err := executor(ctx, r.pool).Exec(ctx, query, args...); err != nil {
		if isIdempotencyKeyConflict(err) {
			return ErrIdempotencyKeyExists
		}
		return fmt.Errorf("insert idempotency key: %w", err)
	}
	return nil
}

func (r *IdempotencyRepository) Get(ctx context.Context, key uuid.UUID) (IdempotencyRecord, error) {
	ctx, cancel := withTimeout(ctx, r.queryTimeout)
	defer cancel()

	query, args, err := sb.Select("idempotency_key", "request_hash", "trip_id", "created_at", "expires_at").
		From("idempotency_keys").
		Where(squirrel.Eq{"idempotency_key": key}).
		ToSql()
	if err != nil {
		return IdempotencyRecord{}, fmt.Errorf("build select idempotency key: %w", err)
	}

	var rec IdempotencyRecord
	err = executor(ctx, r.pool).QueryRow(ctx, query, args...).Scan(
		&rec.Key,
		&rec.RequestHash,
		&rec.TripID,
		&rec.CreatedAt,
		&rec.ExpiresAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return IdempotencyRecord{}, ErrIdempotencyKeyNotFound
	}
	if err != nil {
		return IdempotencyRecord{}, fmt.Errorf("select idempotency key: %w", err)
	}
	rec.CreatedAt = rec.CreatedAt.UTC()
	rec.ExpiresAt = rec.ExpiresAt.UTC()
	return rec, nil
}

// DeleteExact удаляет строку только с тем сроком, который увидел вызывающий.
// Чужую, уже обновлённую запись это не затрёт.
func (r *IdempotencyRepository) DeleteExact(ctx context.Context, key uuid.UUID, expiresAt time.Time) (bool, error) {
	ctx, cancel := withTimeout(ctx, r.queryTimeout)
	defer cancel()

	query, args, err := sb.Delete("idempotency_keys").
		Where(squirrel.Eq{"idempotency_key": key, "expires_at": expiresAt}).
		ToSql()
	if err != nil {
		return false, fmt.Errorf("build delete idempotency key: %w", err)
	}
	tag, err := executor(ctx, r.pool).Exec(ctx, query, args...)
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
