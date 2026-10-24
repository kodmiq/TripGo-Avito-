package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/kodmiq/TripGo-Avito-/internal/generated"
)

const maxBodyBytes = 1 << 20

// ValidationError — запрос не прошёл проверку до бизнес-логики.
type ValidationError struct {
	Detail string
}

func (e *ValidationError) Error() string {
	return e.Detail
}

func decodeCreateTrip(r *http.Request) (CreateTrip, error) {
	if err := requireJSON(r); err != nil {
		return CreateTrip{}, err
	}
	raw, err := readBody(r)
	if err != nil {
		return CreateTrip{}, err
	}
	if len(raw) == 0 {
		return CreateTrip{}, &ValidationError{Detail: "request body is required"}
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var body api.TripData
	if err := dec.Decode(&body); err != nil {
		return CreateTrip{}, invalidBody(err)
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return CreateTrip{}, &ValidationError{Detail: "request body must contain a single JSON object"}
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return CreateTrip{}, &ValidationError{Detail: "Request validation failed"}
	}
	for _, key := range []string{"user_id", "driver_id", "start_point", "end_point", "price"} {
		if _, ok := obj[key]; !ok {
			return CreateTrip{}, &ValidationError{Detail: key + " is required"}
		}
	}
	if err := requireCoordinates(obj["start_point"], "start_point"); err != nil {
		return CreateTrip{}, err
	}
	if err := requireCoordinates(obj["end_point"], "end_point"); err != nil {
		return CreateTrip{}, err
	}
	if body.UserId == uuid.Nil {
		return CreateTrip{}, &ValidationError{Detail: "user_id must be a non-empty UUID"}
	}
	if body.DriverId == uuid.Nil {
		return CreateTrip{}, &ValidationError{Detail: "driver_id must be a non-empty UUID"}
	}
	if err := validateCoordinates(body.StartPoint.Latitude, body.StartPoint.Longitude, "start_point"); err != nil {
		return CreateTrip{}, err
	}
	if err := validateCoordinates(body.EndPoint.Latitude, body.EndPoint.Longitude, "end_point"); err != nil {
		return CreateTrip{}, err
	}
	if body.Price < 0 {
		return CreateTrip{}, &ValidationError{Detail: "price must be greater than or equal to 0"}
	}

	return CreateTrip{
		UserID:   body.UserId,
		DriverID: body.DriverId,
		StartLat: body.StartPoint.Latitude,
		StartLon: body.StartPoint.Longitude,
		EndLat:   body.EndPoint.Latitude,
		EndLon:   body.EndPoint.Longitude,
		Price:    body.Price,
	}, nil
}

func rejectUnexpectedBody(r *http.Request) error {
	raw, err := readBody(r)
	if err != nil {
		return err
	}
	if len(raw) == 0 {
		return nil
	}
	return &ValidationError{Detail: "request body must be empty"}
}

func requireJSON(r *http.Request) error {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return &ValidationError{Detail: "Content-Type must be application/json"}
	}
	return nil
}

func readBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	defer r.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes+1))
	if err != nil {
		return nil, &ValidationError{Detail: "failed to read request body"}
	}
	if len(raw) > maxBodyBytes {
		return nil, &ValidationError{Detail: "request body is too large"}
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil, nil
	}
	return raw, nil
}

func requireCoordinates(raw json.RawMessage, name string) error {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return &ValidationError{Detail: name + " must be an object"}
	}
	if _, ok := obj["latitude"]; !ok {
		return &ValidationError{Detail: name + ".latitude is required"}
	}
	if _, ok := obj["longitude"]; !ok {
		return &ValidationError{Detail: name + ".longitude is required"}
	}
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

func invalidBody(err error) error {
	message := err.Error()
	switch {
	case strings.Contains(message, "unknown field"):
		return &ValidationError{Detail: "unknown field in request body"}
	case strings.Contains(message, "cannot unmarshal"), strings.Contains(message, "unexpected EOF"):
		return &ValidationError{Detail: "Request validation failed"}
	default:
		return &ValidationError{Detail: "Request validation failed"}
	}
}
