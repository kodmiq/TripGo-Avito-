package finish

import (
	"bytes"
	"io"
	"net/http"

	domain "github.com/kodmiq/TripGo-Avito-/internal/domain/trip"
)

const maxBodyBytes = 1 << 20

func rejectUnexpectedBody(r *http.Request) error {
	raw, err := readBody(r)
	if err != nil {
		return err
	}
	if len(raw) == 0 {
		return nil
	}
	return &domain.ValidationError{Detail: "request body must be empty"}
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
