package trip

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kodmiq/TripGo-Avito-/internal/model"
)

func TestStaleQueryKeepsTimePredicateInSQL(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	cursor := &model.StaleCursor{
		SilentSince: now.Add(-2 * time.Minute),
		ID:          uuid.MustParse("11111111-1111-1111-1111-111111111111"),
	}

	query, args, err := staleQuery(now, time.Minute, 100, cursor)
	if err != nil {
		t.Fatalf("staleQuery() = %v", err)
	}
	if !strings.Contains(query, "COALESCE(last_position_at, started_at)") {
		t.Fatalf("query lost the silence expression: %s", query)
	}
	if !strings.Contains(query, "interval '1 microsecond'") {
		t.Fatalf("time threshold is not computed in SQL: %s", query)
	}
	if !strings.Contains(query, "status") || !strings.Contains(query, "ORDER BY") {
		t.Fatalf("query = %s", query)
	}
	if !strings.Contains(query, "> ($4, $5)") {
		t.Fatalf("cursor is not a keyset predicate: %s", query)
	}
	if len(args) != 5 {
		t.Fatalf("args = %d, query = %s", len(args), query)
	}
}
