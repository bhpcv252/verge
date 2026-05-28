package observability

import "go.opentelemetry.io/otel"

func Noop() *Provider {
	noopMeter := otel.GetMeterProvider().Meter("verge")

	m, _ := newMetrics(noopMeter)

	return &Provider{
		Tracer:  otel.GetTracerProvider().Tracer("verge"),
		Meter:   noopMeter,
		Logger:  newLogger("info"),
		Metrics: m,
	}
}
