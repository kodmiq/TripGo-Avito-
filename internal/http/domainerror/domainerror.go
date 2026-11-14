package domainerror

import (
	"errors"
	"net/http"

	domain "github.com/kodmiq/TripGo-Avito-/internal/domain/trip"
	"github.com/kodmiq/TripGo-Avito-/internal/generated"
)

const (
	codeInvalidRequest      = "invalid_request"
	codeDriverBusy          = "driver_busy"
	codeTripCompleted       = "trip_completed"
	codeIdempotencyConflict = "idempotency_conflict"
	codeTripNotFound        = "trip_not_found"
	codeInternalError       = "internal_error"
)

// Problem переводит доменную ошибку в HTTP-ответ. Хендлеры этот switch не дублируют.
// Текст неизвестной ошибки в тело не попадает.
func Problem(err error) (int, api.Problem) {
	var validation *domain.ValidationError
	switch {
	case errors.As(err, &validation):
		return problem(http.StatusBadRequest, codeInvalidRequest, "Invalid request",
			"https://tripgo.example/problems/invalid-request", validation.Detail)
	case errors.Is(err, domain.ErrDriverBusy):
		return problem(http.StatusConflict, codeDriverBusy, "Driver busy",
			"https://tripgo.example/problems/driver-busy", "Driver already has an active trip")
	case errors.Is(err, domain.ErrTripCompleted):
		return problem(http.StatusConflict, codeTripCompleted, "Trip completed",
			"https://tripgo.example/problems/trip-completed", "Operation is not allowed for a completed trip")
	case errors.Is(err, domain.ErrIdempotencyConflict):
		return problem(http.StatusConflict, codeIdempotencyConflict, "Idempotency conflict",
			"https://tripgo.example/problems/idempotency-conflict", "Idempotency-Key was already used with a different request body")
	case errors.Is(err, domain.ErrTripNotFound):
		return problem(http.StatusNotFound, codeTripNotFound, "Trip not found",
			"https://tripgo.example/problems/trip-not-found", "Trip was not found")
	default:
		return Internal()
	}
}

// InvalidRequest — тело или параметр не разобрались до вызова usecase.
func InvalidRequest(detail string) (int, api.Problem) {
	return problem(http.StatusBadRequest, codeInvalidRequest, "Invalid request",
		"https://tripgo.example/problems/invalid-request", detail)
}

// Internal — ответ 500 без текста исходной ошибки.
func Internal() (int, api.Problem) {
	return problem(http.StatusInternalServerError, codeInternalError, "Internal Server Error",
		"https://tripgo.example/problems/internal-error", "Internal server error")
}

func problem(status int, code, title, typeURL, detail string) (int, api.Problem) {
	return status, api.Problem{
		Type:   typeURL,
		Title:  title,
		Status: int32(status), //nolint:gosec // G115: код HTTP-ответа помещается в int32
		Detail: &detail,
		Code:   code,
	}
}
