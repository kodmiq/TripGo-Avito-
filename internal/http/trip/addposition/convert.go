package addposition

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/google/uuid"

	domain "github.com/kodmiq/TripGo-Avito-/internal/domain/trip"
	"github.com/kodmiq/TripGo-Avito-/internal/generated"
	"github.com/kodmiq/TripGo-Avito-/internal/model"
)

const (
	maxBodyBytes                  = 1 << 20
	detailRequestValidationFailed = "Request validation failed"
)

func decodePosition(r *http.Request, tripID uuid.UUID) (model.NewPosition, error) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return model.NewPosition{}, &domain.ValidationError{Detail: "Content-Type must be application/json"}
	}
	raw, err := readBody(r)
	if err != nil {
		return model.NewPosition{}, err
	}
	if len(raw) == 0 {
		return model.NewPosition{}, &domain.ValidationError{Detail: "request body is required"}
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var body api.PositionData
	if err := dec.Decode(&body); err != nil {
		return model.NewPosition{}, invalidBody(err)
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return model.NewPosition{}, &domain.ValidationError{Detail: "request body must contain a single JSON object"}
	}
	if err := rejectNullFields(raw); err != nil {
		return model.NewPosition{}, err
	}

	return model.NewPosition{
		TripID:     tripID,
		Latitude:   body.Coordinates.Latitude,
		Longitude:  body.Coordinates.Longitude,
		RecordedAt: body.RecordedAt,
	}, nil
}

func readBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	defer func() {
		_ = r.Body.Close()
	}()
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes+1))
	if err != nil {
		return nil, &domain.ValidationError{Detail: "failed to read request body"}
	}
	if len(raw) > maxBodyBytes {
		return nil, &domain.ValidationError{Detail: "request body is too large"}
	}
	return bytes.TrimSpace(raw), nil
}

func rejectNullFields(raw []byte) error {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return &domain.ValidationError{Detail: detailRequestValidationFailed}
	}
	for _, key := range []string{"coordinates", "recorded_at"} {
		value, ok := obj[key]
		if !ok {
			return &domain.ValidationError{Detail: key + " is required"}
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return &domain.ValidationError{Detail: key + " must not be null"}
		}
	}
	var coordinates map[string]json.RawMessage
	if err := json.Unmarshal(obj["coordinates"], &coordinates); err != nil {
		return &domain.ValidationError{Detail: "coordinates must be an object"}
	}
	for _, axis := range []string{"latitude", "longitude"} {
		value, ok := coordinates[axis]
		if !ok {
			return &domain.ValidationError{Detail: "coordinates." + axis + " is required"}
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return &domain.ValidationError{Detail: "coordinates." + axis + " must not be null"}
		}
	}
	return nil
}

func invalidBody(err error) error {
	message := err.Error()
	switch {
	case strings.Contains(message, "unknown field"):
		return &domain.ValidationError{Detail: "unknown field in request body"}
	default:
		return &domain.ValidationError{Detail: detailRequestValidationFailed}
	}
}
