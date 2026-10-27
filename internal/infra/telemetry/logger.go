package telemetry

import (
	"log/slog"
	"os"
)

const serviceName = "trip-service"

// NewLogger пишет JSON в stdout. Уровень приходит из конфигурации.
// trace_id и span_id добавляет обработчик, когда запись сделана внутри спана.
func NewLogger(level slog.Level) *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})).With("service", serviceName)
}
