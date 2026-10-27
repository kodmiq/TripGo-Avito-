package create

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"

	domain "github.com/kodmiq/TripGo-Avito-/internal/domain/trip"
	"github.com/kodmiq/TripGo-Avito-/internal/generated"
	"github.com/kodmiq/TripGo-Avito-/internal/model"
)

const maxBodyBytes = 1 << 20

func decodeCreateTrip(r *http.Request) (model.CreateTrip, error) {
	if err := requireJSON(r); err != nil {
		return model.CreateTrip{}, err
	}
	raw, err := readBody(r)
	if err != nil {
		return model.CreateTrip{}, err
	}
	if len(raw) == 0 {
		return model.CreateTrip{}, &domain.ValidationError{Detail: "request body is required"}
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var body api.TripData
	if err := dec.Decode(&body); err != nil {
		return model.CreateTrip{}, invalidBody(err)
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return model.CreateTrip{}, &domain.ValidationError{Detail: "request body must contain a single JSON object"}
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return model.CreateTrip{}, &domain.ValidationError{Detail: "Request validation failed"}
	}
	for _, key := range []string{"user_id", "driver_id", "start_point", "end_point", "price"} {
		if _, ok := obj[key]; !ok {
			return model.CreateTrip{}, &domain.ValidationError{Detail: key + " is required"}
		}
	}
	if err := requireCoordinates(obj["start_point"], "start_point"); err != nil {
		return model.CreateTrip{}, err
	}
	if err := requireCoordinates(obj["end_point"], "end_point"); err != nil {
		return model.CreateTrip{}, err
	}

	in := model.CreateTrip{
		UserID:   body.UserId,
		DriverID: body.DriverId,
		StartLat: body.StartPoint.Latitude,
		StartLon: body.StartPoint.Longitude,
		EndLat:   body.EndPoint.Latitude,
		EndLon:   body.EndPoint.Longitude,
		Price:    body.Price,
	}
	if err := domain.ValidateCreate(in.UserID, in.DriverID, in.StartLat, in.StartLon, in.EndLat, in.EndLon, in.Price); err != nil {
		return model.CreateTrip{}, err
	}
	return in, nil
}

func requireJSON(r *http.Request) error {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return &domain.ValidationError{Detail: "Content-Type must be application/json"}
	}
	return nil
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
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil, nil
	}
	return raw, nil
}

func requireCoordinates(raw json.RawMessage, name string) error {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return &domain.ValidationError{Detail: name + " must be an object"}
	}
	if _, ok := obj["latitude"]; !ok {
		return &domain.ValidationError{Detail: name + ".latitude is required"}
	}
	if _, ok := obj["longitude"]; !ok {
		return &domain.ValidationError{Detail: name + ".longitude is required"}
	}
	return nil
}

func invalidBody(err error) error {
	message := err.Error()
	switch {
	case strings.Contains(message, "unknown field"):
		return &domain.ValidationError{Detail: "unknown field in request body"}
	case strings.Contains(message, "cannot unmarshal"), strings.Contains(message, "unexpected EOF"):
		return &domain.ValidationError{Detail: "Request validation failed"}
	default:
		return &domain.ValidationError{Detail: "Request validation failed"}
	}
}
