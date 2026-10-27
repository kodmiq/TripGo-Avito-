package trip

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusActive    Status = "active"
	StatusCompleted Status = "completed"
)

// Trip — поездка и правила её переходов. Поля меняет сам тип, не вызывающий код.
type Trip struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	DriverID   uuid.UUID
	StartLat   float64
	StartLon   float64
	EndLat     float64
	EndLon     float64
	Price      int64
	Status     Status
	StartedAt  time.Time
	FinishedAt *time.Time
}

// NewActive собирает активную поездку. id и startedAt назначает вызывающий код.
func NewActive(id, userID, driverID uuid.UUID, startLat, startLon, endLat, endLon float64, price int64, startedAt time.Time) (Trip, error) {
	if err := ValidateCreate(userID, driverID, startLat, startLon, endLat, endLon, price); err != nil {
		return Trip{}, err
	}
	return Trip{
		ID:        id,
		UserID:    userID,
		DriverID:  driverID,
		StartLat:  startLat,
		StartLon:  startLon,
		EndLat:    endLat,
		EndLon:    endLon,
		Price:     price,
		Status:    StatusActive,
		StartedAt: startedAt,
	}, nil
}

// ValidateCreate проверяет поля создания поездки.
func ValidateCreate(userID, driverID uuid.UUID, startLat, startLon, endLat, endLon float64, price int64) error {
	if userID == uuid.Nil {
		return &ValidationError{Detail: "user_id must be a non-empty UUID"}
	}
	if driverID == uuid.Nil {
		return &ValidationError{Detail: "driver_id must be a non-empty UUID"}
	}
	if err := validateCoordinates(startLat, startLon, "start_point"); err != nil {
		return err
	}
	if err := validateCoordinates(endLat, endLon, "end_point"); err != nil {
		return err
	}
	if price < 0 {
		return &ValidationError{Detail: "price must be greater than or equal to 0"}
	}
	return nil
}

// Finish переводит активную поездку в completed. База и контекст здесь не нужны:
// гонку двух завершений закрывает условный UPDATE в репозитории.
func (t *Trip) Finish(at time.Time) error {
	if t.Status != StatusActive {
		return ErrTripCompleted
	}
	t.Status = StatusCompleted
	finishedAt := at
	t.FinishedAt = &finishedAt
	return nil
}

func validateCoordinates(latitude, longitude float64, name string) error {
	if latitude < -90 || latitude > 90 {
		return &ValidationError{Detail: fmt.Sprintf("%s.latitude must be between -90 and 90", name)}
	}
	if longitude < -180 || longitude > 180 {
		return &ValidationError{Detail: fmt.Sprintf("%s.longitude must be between -180 and 180", name)}
	}
	return nil
}
