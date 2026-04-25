package telemetry

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Init configures a global TracerProvider when OTEL_EXPORTER_OTLP_ENDPOINT is set.
// Returns shutdown function (safe to call with nil provider if disabled).
func Init(ctx context.Context, log zerolog.Logger, serviceName string) (func(context.Context) error, error) {
	endpoint := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	if endpoint == "" {
		return func(context.Context) error { return nil }, nil
	}
	// otlptracehttp.WithEndpoint expects host:port (no scheme, no path).
	endpoint = strings.TrimPrefix(endpoint, "http://")
	endpoint = strings.TrimPrefix(endpoint, "https://")
	if i := strings.Index(endpoint, "/"); i >= 0 {
		endpoint = endpoint[:i]
	}
	if serviceName == "" {
		serviceName = "api-gateway"
	}

	opts := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(endpoint),
		otlptracehttp.WithURLPath("/v1/traces"),
	}
	if strings.EqualFold(os.Getenv("OTEL_EXPORTER_OTLP_INSECURE"), "true") {
		opts = append(opts, otlptracehttp.WithInsecure())
	}

	exp, err := otlptracehttp.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("otlp trace exporter: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
		resource.WithProcess(),
		resource.WithOS(),
		resource.WithHost(),
		resource.WithAttributes(semconv.ServiceName(serviceName)),
	)
	if err != nil {
		return nil, fmt.Errorf("otel resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	log.Info().Str("endpoint", endpoint).Msg("OpenTelemetry OTLP tracing enabled")

	return func(shutdownCtx context.Context) error {
		ctx, cancel := context.WithTimeout(shutdownCtx, 10*time.Second)
		defer cancel()
		if err := tp.Shutdown(ctx); err != nil {
			return fmt.Errorf("tracer shutdown: %w", err)
		}
		return nil
	}, nil
}
