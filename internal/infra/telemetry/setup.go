package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"strings"

	"github.com/kodmiq/TripGo-Avito-/internal/config"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Providers — логгер, трейсы и метрики процесса. Экспорт идёт по OTLP;
// недоступный коллектор не мешает запросам, ошибки уходят в Shutdown.
type Providers struct {
	Logger   *slog.Logger
	Tracer   *sdktrace.TracerProvider
	Meter    *metric.MeterProvider
	DB       *DBTracer
	Trips    Trips
	shutdown func(context.Context) error
}

func (p Providers) Shutdown(ctx context.Context) error {
	if p.shutdown == nil {
		return nil
	}
	return p.shutdown(ctx)
}

// Setup собирает SDK. Адрес коллектора читают экспортёры из окружения:
// OTEL_EXPORTER_OTLP_ENDPOINT — базовый URL, путь сигнала SDK дописывает сам.
func Setup(ctx context.Context, cfg config.Config) (Providers, error) {
	log := NewLogger(cfg.LogLevel, cfg.OTELServiceName)

	res, err := serviceResource(cfg)
	if err != nil {
		return Providers{}, err
	}
	sampler, err := traceSampler(cfg.OTELTracesSampler)
	if err != nil {
		return Providers{}, err
	}
	traceExp, err := newTraceExporter(ctx, cfg.OTELProtocol)
	if err != nil {
		return Providers{}, err
	}
	metricExp, err := newMetricExporter(ctx, cfg.OTELProtocol)
	if err != nil {
		return Providers{}, err
	}

	tracer := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExp),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)
	reader := metric.NewPeriodicReader(metricExp)
	meterProvider := metric.NewMeterProvider(
		metric.WithReader(reader),
		metric.WithResource(res),
	)
	meter := meterProvider.Meter("trip-service")

	dbTracer, err := NewDBTracer(tracer, meter)
	if err != nil {
		return Providers{}, err
	}
	trips, err := newTrips(meter)
	if err != nil {
		return Providers{}, err
	}

	return Providers{
		Logger: log,
		Tracer: tracer,
		Meter:  meterProvider,
		DB:     dbTracer,
		Trips:  trips,
		shutdown: func(ctx context.Context) error {
			traceErr := tracer.Shutdown(ctx)
			metricErr := meterProvider.Shutdown(ctx)
			if traceErr != nil && metricErr != nil {
				return fmt.Errorf("shutdown telemetry: traces: %w; metrics: %v", traceErr, metricErr)
			}
			if traceErr != nil {
				return fmt.Errorf("shutdown traces: %w", traceErr)
			}
			if metricErr != nil {
				return fmt.Errorf("shutdown metrics: %w", metricErr)
			}
			return nil
		},
	}, nil
}

func serviceResource(cfg config.Config) (*resource.Resource, error) {
	attrs, err := resourceAttributes(cfg.OTELResourceAttributes)
	if err != nil {
		return nil, err
	}
	attrs = append(attrs,
		attribute.String("service.name", cfg.OTELServiceName),
		attribute.String("service.version", buildVersion()),
	)
	return resource.NewWithAttributes("", attrs...), nil
}

func resourceAttributes(raw string) ([]attribute.KeyValue, error) {
	var attrs []attribute.KeyValue
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, value, ok := strings.Cut(part, "=")
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if !ok || key == "" {
			return nil, fmt.Errorf("OTEL_RESOURCE_ATTRIBUTES entry %q must be key=value", part)
		}
		attrs = append(attrs, attribute.String(key, value))
	}
	if len(attrs) == 0 {
		return nil, fmt.Errorf("OTEL_RESOURCE_ATTRIBUTES must contain at least one key=value")
	}
	return attrs, nil
}

func traceSampler(name string) (sdktrace.Sampler, error) {
	switch name {
	case "always_on":
		return sdktrace.AlwaysSample(), nil
	case "always_off":
		return sdktrace.NeverSample(), nil
	case "parentbased_always_on":
		return sdktrace.ParentBased(sdktrace.AlwaysSample()), nil
	case "parentbased_always_off":
		return sdktrace.ParentBased(sdktrace.NeverSample()), nil
	default:
		return nil, fmt.Errorf("OTEL_TRACES_SAMPLER %q is not supported", name)
	}
}

func newTraceExporter(ctx context.Context, protocol string) (*otlptrace.Exporter, error) {
	switch protocol {
	case "http/protobuf":
		return otlptracehttp.New(ctx)
	case "grpc":
		return otlptracegrpc.New(ctx)
	default:
		return nil, fmt.Errorf("OTEL_EXPORTER_OTLP_PROTOCOL %q is not supported", protocol)
	}
}

func newMetricExporter(ctx context.Context, protocol string) (metric.Exporter, error) {
	switch protocol {
	case "http/protobuf":
		return otlpmetrichttp.New(ctx)
	case "grpc":
		return otlpmetricgrpc.New(ctx)
	default:
		return nil, fmt.Errorf("OTEL_EXPORTER_OTLP_PROTOCOL %q is not supported", protocol)
	}
}

func buildVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Version == "" {
		return "dev"
	}
	return info.Main.Version
}
