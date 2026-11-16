package positions

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	"github.com/kodmiq/TripGo-Avito-/internal/model"
)

const (
	resultQueued    = "queued"
	resultDuplicate = "duplicate"
	resultDropped   = "dropped"
	resultSuccess   = "success"
	resultError     = "error"
	resultCancelled = "cancelled"
)

type (
	tripLister interface {
		ListStale(ctx context.Context, now time.Time, staleAfter time.Duration, limit int, cursor *model.StaleCursor) ([]model.StaleTrip, error)
	}

	pushGateway interface {
		RequestPosition(ctx context.Context, driverID, tripID uuid.UUID) error
	}

	metrics interface {
		StaleCheck(ctx context.Context, result string)
		RowsRead(ctx context.Context, n int)
		Request(ctx context.Context, result string, duration time.Duration)
	}
)

// Ticker прячет time.Ticker, чтобы тесты присылали тики сами и не ждали интервал.
type Ticker interface {
	C() <-chan time.Time
	Stop()
}

type clockTicker struct {
	*time.Ticker
}

func (t clockTicker) C() <-chan time.Time { return t.Ticker.C }

// task — одна поездка в очереди. link связывает обработку со спаном проверки.
type task struct {
	tripID      uuid.UUID
	driverID    uuid.UUID
	silentSince time.Time
	link        trace.SpanContext
	index       int
}
