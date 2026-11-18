//go:build integration

package trip_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	domain "github.com/kodmiq/TripGo-Avito-/internal/domain/trip"
	"github.com/kodmiq/TripGo-Avito-/internal/infra/postgres/transaction"
	"github.com/kodmiq/TripGo-Avito-/internal/model"
	"github.com/kodmiq/TripGo-Avito-/internal/repository/position"
	triprepo "github.com/kodmiq/TripGo-Avito-/internal/repository/trip"
)

func TestPositionSaveAdvancesAndRollsBack(t *testing.T) {
	ctx := context.Background()
	trips := triprepo.New(pool, queryTimeout)
	positions := position.New(pool, queryTimeout)
	tx := transaction.New(pool, queryTimeout, slog.New(slog.DiscardHandler))
	draft := sample(uuid.New())
	cleanup(t, draft.ID)
	if _, err := trips.Save(ctx, draft); err != nil {
		t.Fatalf("save trip: %v", err)
	}

	newer := draft.StartedAt.Add(2 * time.Minute)
	older := draft.StartedAt.Add(time.Minute)
	if err := savePosition(ctx, tx, positions, draft.ID, 59.93, 30.33, newer); err != nil {
		t.Fatalf("save newer: %v", err)
	}
	if err := savePosition(ctx, tx, positions, draft.ID, 59.94, 30.34, older); err != nil {
		t.Fatalf("save older: %v", err)
	}

	got, err := trips.Get(ctx, draft.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.LastPositionAt == nil || !got.LastPositionAt.Equal(newer.UTC()) {
		t.Fatalf("last_position_at = %v, want %v", got.LastPositionAt, newer)
	}

	route, err := positions.List(ctx, draft.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(route) != 2 || !route[0].RecordedAt.Equal(older.UTC()) || !route[1].RecordedAt.Equal(newer.UTC()) {
		t.Fatalf("route = %+v", route)
	}

	forced := errors.New("forced rollback")
	err = tx.Do(ctx, func(ctx context.Context) error {
		_, saveErr := positions.Save(ctx, model.NewPosition{
			TripID: draft.ID, Latitude: 1, Longitude: 2, RecordedAt: newer.Add(time.Minute),
		}, newer)
		if saveErr != nil {
			return saveErr
		}
		return forced
	})
	if !errors.Is(err, forced) {
		t.Fatalf("Do() = %v", err)
	}

	route, err = positions.List(ctx, draft.ID)
	if err != nil {
		t.Fatalf("list after rollback: %v", err)
	}
	if len(route) != 2 {
		t.Fatalf("positions after rollback = %d", len(route))
	}
	got, err = trips.Get(ctx, draft.ID)
	if err != nil {
		t.Fatalf("get after rollback: %v", err)
	}
	if got.LastPositionAt == nil || !got.LastPositionAt.Equal(newer.UTC()) {
		t.Fatalf("last_position_at after rollback = %v", got.LastPositionAt)
	}
}

func TestPositionSaveRejectsMissingAndCompleted(t *testing.T) {
	ctx := context.Background()
	trips := triprepo.New(pool, queryTimeout)
	positions := position.New(pool, queryTimeout)
	tx := transaction.New(pool, queryTimeout, slog.New(slog.DiscardHandler))

	err := savePosition(ctx, tx, positions, uuid.New(), 1, 2, time.Now().UTC())
	if !errors.Is(err, domain.ErrTripNotFound) {
		t.Fatalf("missing trip = %v", err)
	}

	draft := sample(uuid.New())
	cleanup(t, draft.ID)
	if _, err := trips.Save(ctx, draft); err != nil {
		t.Fatalf("save trip: %v", err)
	}
	if _, _, err := trips.MarkCompleted(ctx, draft.ID, draft.StartedAt.Add(time.Minute)); err != nil {
		t.Fatalf("finish: %v", err)
	}
	err = savePosition(ctx, tx, positions, draft.ID, 1, 2, draft.StartedAt.Add(2*time.Minute))
	if !errors.Is(err, domain.ErrTripCompleted) {
		t.Fatalf("completed trip = %v", err)
	}
	route, err := positions.List(ctx, draft.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(route) != 0 {
		t.Fatalf("completed trip stored %d positions", len(route))
	}
}

func TestListStaleSkipsFreshAndCompleted(t *testing.T) {
	ctx := context.Background()
	trips := triprepo.New(pool, queryTimeout)
	positions := position.New(pool, queryTimeout)
	tx := transaction.New(pool, queryTimeout, slog.New(slog.DiscardHandler))
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)

	silent := tripAt(t, trips, now.Add(-10*time.Minute))
	freshStart := tripAt(t, trips, now.Add(-30*time.Second))
	freshPosition := tripAt(t, trips, now.Add(-10*time.Minute))
	completed := tripAt(t, trips, now.Add(-10*time.Minute))
	older := tripAt(t, trips, now.Add(-30*time.Minute))

	if err := savePosition(ctx, tx, positions, freshPosition.ID, 59.9, 30.3, now.Add(-20*time.Second)); err != nil {
		t.Fatalf("fresh position: %v", err)
	}
	if _, _, err := trips.MarkCompleted(ctx, completed.ID, now); err != nil {
		t.Fatalf("finish: %v", err)
	}

	got, err := trips.ListStale(ctx, now, time.Minute, 100, nil)
	if err != nil {
		t.Fatalf("ListStale() = %v", err)
	}
	ids := map[uuid.UUID]int{}
	var matched []uuid.UUID
	for _, trip := range got {
		ids[trip.ID]++
		switch trip.ID {
		case older.ID, silent.ID:
			matched = append(matched, trip.ID)
		}
	}
	if ids[freshStart.ID] != 0 || ids[freshPosition.ID] != 0 || ids[completed.ID] != 0 {
		t.Fatalf("unexpected trips in scan: %+v", got)
	}
	if len(matched) != 2 || matched[0] != older.ID || matched[1] != silent.ID {
		t.Fatalf("order = %v, want %s then %s", matched, older.ID, silent.ID)
	}
}

func TestListStaleKeysetDoesNotSkipOrRepeat(t *testing.T) {
	ctx := context.Background()
	trips := triprepo.New(pool, queryTimeout)
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	same := now.Add(-5 * time.Minute)
	created := []uuid.UUID{
		tripAt(t, trips, same).ID,
		tripAt(t, trips, same).ID,
		tripAt(t, trips, same).ID,
	}

	seen := map[uuid.UUID]int{}
	var cursor *model.StaleCursor
	for page := 0; page < 5 && len(seen) < len(created); page++ {
		batch, err := trips.ListStale(ctx, now, time.Minute, 2, cursor)
		if err != nil {
			t.Fatalf("page %d: %v", page, err)
		}
		if len(batch) == 0 {
			break
		}
		for _, trip := range batch {
			for _, id := range created {
				if trip.ID == id {
					seen[id]++
				}
			}
		}
		last := batch[len(batch)-1]
		cursor = &model.StaleCursor{SilentSince: last.SilentSince, ID: last.ID}
	}
	for _, id := range created {
		if seen[id] != 1 {
			t.Fatalf("trip %s seen %d times", id, seen[id])
		}
	}
}

func TestStalePlanUsesIndex(t *testing.T) {
	ctx := context.Background()
	freshAt := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	staleAt := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	t.Cleanup(func() {
		cleanCtx, cancel := context.WithTimeout(context.Background(), queryTimeout)
		defer cancel()
		if _, err := pool.Exec(cleanCtx, "DELETE FROM trips WHERE started_at = $1 OR started_at = $2", freshAt, staleAt); err != nil {
			t.Errorf("cleanup plan trips: %v", err)
		}
	})

	if _, err := pool.Exec(ctx, `
		INSERT INTO trips (
			id, user_id, driver_id,
			start_latitude, start_longitude, end_latitude, end_longitude,
			price, status, started_at
		)
		SELECT gen_random_uuid(), gen_random_uuid(), gen_random_uuid(),
			59.9, 30.3, 59.8, 30.4,
			100, 'active', $1
		FROM generate_series(1, 3000)`, freshAt); err != nil {
		t.Fatalf("insert fresh: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO trips (
			id, user_id, driver_id,
			start_latitude, start_longitude, end_latitude, end_longitude,
			price, status, started_at
		)
		SELECT gen_random_uuid(), gen_random_uuid(), gen_random_uuid(),
			59.9, 30.3, 59.8, 30.4,
			100, 'active', $1
		FROM generate_series(1, 50)`, staleAt); err != nil {
		t.Fatalf("insert stale: %v", err)
	}
	if _, err := pool.Exec(ctx, "ANALYZE trips"); err != nil {
		t.Fatalf("analyze: %v", err)
	}

	rows, err := pool.Query(ctx, `
		EXPLAIN (ANALYZE, FORMAT TEXT)
		SELECT id, driver_id, COALESCE(last_position_at, started_at)
		FROM trips
		WHERE status = 'active'
		  AND COALESCE(last_position_at, started_at) < $1::timestamptz - ($2::bigint * interval '1 microsecond')
		  AND (COALESCE(last_position_at, started_at), id) > ($3, $4)
		ORDER BY COALESCE(last_position_at, started_at) ASC, id ASC
		LIMIT 100`,
		freshAt,
		time.Hour.Microseconds(),
		staleAt.Add(-time.Second),
		uuid.MustParse("00000000-0000-0000-0000-000000000000"),
	)
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	defer rows.Close()

	var plan strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatalf("scan plan: %v", err)
		}
		plan.WriteString(line)
		plan.WriteByte('\n')
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("plan rows: %v", err)
	}
	text := plan.String()
	if strings.Contains(text, "Seq Scan") || !strings.Contains(text, "Index") {
		t.Fatalf("plan = %s", text)
	}
}

func tripAt(t *testing.T, trips *triprepo.Repository, started time.Time) model.Trip {
	t.Helper()
	draft := sample(uuid.New())
	draft.StartedAt = started.UTC()
	cleanup(t, draft.ID)
	stored, err := trips.Save(context.Background(), draft)
	if err != nil {
		t.Fatalf("save trip: %v", err)
	}
	return stored
}

func savePosition(
	ctx context.Context,
	tx *transaction.Manager,
	positions *position.Repository,
	tripID uuid.UUID,
	lat, lon float64,
	recordedAt time.Time,
) error {
	var stored model.Position
	err := tx.Do(ctx, func(ctx context.Context) error {
		row, err := positions.Save(ctx, model.NewPosition{
			TripID: tripID, Latitude: lat, Longitude: lon, RecordedAt: recordedAt,
		}, recordedAt)
		if err != nil {
			return err
		}
		stored = row
		return nil
	})
	if err != nil {
		return err
	}
	if stored.TripID != tripID {
		return fmt.Errorf("saved position trip_id = %s", stored.TripID)
	}
	return nil
}
