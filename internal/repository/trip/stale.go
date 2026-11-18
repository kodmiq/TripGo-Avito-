package trip

import (
	"context"
	"fmt"
	"time"

	"github.com/Masterminds/squirrel"

	domain "github.com/kodmiq/TripGo-Avito-/internal/domain/trip"
	"github.com/kodmiq/TripGo-Avito-/internal/infra/postgres"
	"github.com/kodmiq/TripGo-Avito-/internal/model"
)

// silentSinceExpr совпадает с выражением индекса trips_active_silent_since_idx.
const silentSinceExpr = "COALESCE(last_position_at, started_at)"

// ListStale отдаёт активные поездки, которые молчат дольше staleAfter.
// Порог вычитается в SQL. Порция продолжается строго после cursor, без OFFSET:
// одинаковое время у соседних поездок не теряет и не дублирует строки.
// Порядок — от тех, кто молчит дольше всех.
func (r *Repository) ListStale(
	ctx context.Context,
	now time.Time,
	staleAfter time.Duration,
	limit int,
	cursor *model.StaleCursor,
) ([]model.StaleTrip, error) {
	if limit < 1 {
		return nil, fmt.Errorf("list stale trips: limit must be positive")
	}
	ctx, cancel := postgres.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	query, args, err := staleQuery(now, staleAfter, limit, cursor)
	if err != nil {
		return nil, err
	}

	rows, err := r.query(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list stale trips: %w", err)
	}
	defer rows.Close()

	out := make([]model.StaleTrip, 0)
	for rows.Next() {
		var trip model.StaleTrip
		if err := rows.Scan(&trip.ID, &trip.DriverID, &trip.SilentSince); err != nil {
			return nil, fmt.Errorf("scan stale trip: %w", err)
		}
		trip.SilentSince = trip.SilentSince.UTC()
		out = append(out, trip)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list stale trips: %w", err)
	}
	return out, nil
}

func staleQuery(now time.Time, staleAfter time.Duration, limit int, cursor *model.StaleCursor) (string, []any, error) {
	olderThan := silentSinceExpr + " < ?::timestamptz - (?::bigint * interval '1 microsecond')"
	if limit < 1 {
		return "", nil, fmt.Errorf("list stale trips: limit must be positive")
	}
	limit64 := uint64(limit) //nolint:gosec // G115: limit уже проверен и это размер порции
	builder := sb.Select("id", "driver_id", silentSinceExpr).
		From("trips").
		Where(squirrel.Eq{"status": string(domain.StatusActive)}).
		Where(squirrel.Expr(olderThan, now, staleAfter.Microseconds())).
		OrderBy(silentSinceExpr+" ASC", "id ASC").
		Limit(limit64)
	if cursor != nil {
		builder = builder.Where(squirrel.Expr("("+silentSinceExpr+", id) > (?, ?)", cursor.SilentSince, cursor.ID))
	}
	query, args, err := builder.ToSql()
	if err != nil {
		return "", nil, fmt.Errorf("build list stale trips: %w", err)
	}
	return query, args, nil
}
