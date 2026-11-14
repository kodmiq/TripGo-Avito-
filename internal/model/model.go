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
	Status     string
	StartedAt  time.Time
	FinishedAt *time.Time
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

// IdempotencyRecord — сохранённый ключ идемпотентности и хеш тела запроса.
type IdempotencyRecord struct {
	Key         uuid.UUID
	RequestHash string
	TripID      uuid.UUID
	CreatedAt   time.Time
	ExpiresAt   time.Time
}
