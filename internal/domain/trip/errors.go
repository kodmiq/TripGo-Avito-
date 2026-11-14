package trip

import "errors"

var (
	ErrTripNotFound        = errors.New("trip not found")
	ErrTripCompleted       = errors.New("trip already completed")
	ErrDriverBusy          = errors.New("driver already has an active trip")
	ErrIdempotencyConflict = errors.New("idempotency conflict")
	// ErrIdempotencyKeyNotFound и ErrIdempotencyKeyExists — сигналы хранилища,
	// а не ответ клиенту. Usecase сам решает, повтор это или новая запись.
	ErrIdempotencyKeyNotFound = errors.New("idempotency key not found")
	ErrIdempotencyKeyExists   = errors.New("idempotency key exists")
)

// ValidationError — поле запроса не проходит доменное правило.
// Detail уходит клиенту как текст problem+json, поэтому без внутренних деталей.
type ValidationError struct {
	Detail string
}

func (e *ValidationError) Error() string {
	return e.Detail
}
