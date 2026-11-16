package model

import (
	"time"

	"github.com/google/uuid"
)

// Trip — поездка, которой обмениваются usecase и репозиторий.
type Trip struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	DriverID   uuid.UUID
	StartLat   float64
	StartLon   float64
	EndLat     float64
	EndLon     float64
	Price      int64
	Status         string
	StartedAt      time.Time
	FinishedAt     *time.Time
	LastPositionAt *time.Time
}

// CreateTrip — поля запроса на создание. id и started_at назначает сервис.
type CreateTrip struct {
	UserID   uuid.UUID
	DriverID uuid.UUID
	StartLat float64
	StartLon float64
	EndLat   float64
	EndLon   float64
	Price    int64
}

// StatusChange — одна запись журнала переходов статуса.
type StatusChange struct {
	TripID     uuid.UUID
	FromStatus *string
	ToStatus   string
	Reason     string
	ChangedAt  time.Time
}

// NewPosition — координата, которую прислало устройство. id назначает база.
type NewPosition struct {
	TripID     uuid.UUID
	Latitude   float64
	Longitude  float64
	RecordedAt time.Time
}

// Position — сохранённая точка маршрута.
type Position struct {
	ID         int64
	TripID     uuid.UUID
	Latitude   float64
	Longitude  float64
	RecordedAt time.Time
}

// StaleTrip — активная поездка, по которой пора запросить координату.
// SilentSince — last_position_at, а если координат не было, started_at.
type StaleTrip struct {
	ID          uuid.UUID
	DriverID    uuid.UUID
	SilentSince time.Time
}

// StaleCursor — последняя строка предыдущей порции. Следующая читается строго после неё.
type StaleCursor struct {
	SilentSince time.Time
	ID          uuid.UUID
}

// IdempotencyRecord — сохранённый ключ идемпотентности и хеш тела запроса.
type IdempotencyRecord struct {
	Key         uuid.UUID
	RequestHash string
	TripID      uuid.UUID
	CreatedAt   time.Time
	ExpiresAt   time.Time
}
