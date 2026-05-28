package observability

import (
	"context"
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	prometheusexporter "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/bhpcv252/verge/internal/config"
)

type Provider struct {
	Tracer trace.Tracer
	Meter  metric.Meter

	Logger *logger

	Metrics *Metrics

	PrometheusHandler http.Handler

	shutdown []func(context.Context) error
}

func (p *Provider) Shutdown(ctx context.Context) error {
	var firstErr error

	for _, fn := range p.shutdown {
		if err := fn(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}

func New(cfg config.OTelConfig) (*Provider, error) {
	log := newLogger(cfg.LogLevel)

	if !cfg.Enabled {
		noopMeter := otel.GetMeterProvider().Meter("verge")

		m, err := newMetrics(noopMeter)
		if err != nil {
			return nil, err
		}

		return &Provider{
			Tracer:  otel.GetTracerProvider().Tracer("verge"),
			Meter:   noopMeter,
			Logger:  log,
			Metrics: m,
		}, nil
	}

	// build service resource metadata
	res, err := resource.Merge(
		resource.Default(),
		resource.NewSchemaless(
			semconv.ServiceNameKey.String(cfg.ServiceName),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("observability: build resource: %w", err)
	}

	p := &Provider{Logger: log}

	// configure tracer provider
	tp, err := buildTracerProvider(cfg, res)
	if err != nil {
		return nil, err
	}

	p.shutdown = append(p.shutdown, tp.Shutdown)

	otel.SetTracerProvider(tp)

	p.Tracer = tp.Tracer("verge")

	// configure meter provider
	mp, promHandler, err := buildMeterProvider(cfg, res)
	if err != nil {
		return nil, err
	}

	p.shutdown = append(p.shutdown, mp.Shutdown)

	otel.SetMeterProvider(mp)

	p.Meter = mp.Meter("verge")
	p.PrometheusHandler = promHandler

	// configure global propagator
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)

	// create metric instruments
	m, err := newMetrics(p.Meter)
	if err != nil {
		return nil, err
	}

	p.Metrics = m

	return p, nil
}

func buildTracerProvider(
	cfg config.OTelConfig,
	res *resource.Resource,
) (*sdktrace.TracerProvider, error) {
	var (
		exp sdktrace.SpanExporter
		err error
	)

	switch cfg.Exporter {
	case "otlp":
		exp, err = otlptracegrpc.New(
			context.Background(),
			otlptracegrpc.WithEndpoint(cfg.OTLPEndpoint),
			otlptracegrpc.WithInsecure(),
		)

	default:
		exp, err = stdouttrace.New(
			stdouttrace.WithPrettyPrint(),
		)
	}

	if err != nil {
		return nil, fmt.Errorf(
			"observability: trace exporter (%s): %w",
			cfg.Exporter,
			err,
		)
	}

	// respect parent sampling decisions
	sampler := sdktrace.ParentBased(
		sdktrace.TraceIDRatioBased(cfg.SampleRate),
	)

	return sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	), nil
}

func buildMeterProvider(
	cfg config.OTelConfig,
	res *resource.Resource,
) (*sdkmetric.MeterProvider, http.Handler, error) {
	var (
		reader      sdkmetric.Reader
		promHandler http.Handler
	)

	switch cfg.Exporter {
	case "otlp":
		exp, err := otlpmetricgrpc.New(
			context.Background(),
			otlpmetricgrpc.WithEndpoint(cfg.OTLPEndpoint),
			otlpmetricgrpc.WithInsecure(),
		)
		if err != nil {
			return nil, nil, fmt.Errorf(
				"observability: OTLP metric exporter: %w",
				err,
			)
		}

		reader = sdkmetric.NewPeriodicReader(
			exp,
			sdkmetric.WithInterval(cfg.MetricsInterval),
		)

	case "prometheus":
		reg := prometheus.NewRegistry()

		exp, err := prometheusexporter.New(
			prometheusexporter.WithRegisterer(reg),
		)
		if err != nil {
			return nil, nil, fmt.Errorf(
				"observability: Prometheus exporter: %w",
				err,
			)
		}

		reader = exp

		promHandler = promhttp.HandlerFor(
			reg,
			promhttp.HandlerOpts{
				EnableOpenMetrics: true,
			},
		)

	default:
		exp, err := stdoutmetric.New(
			stdoutmetric.WithPrettyPrint(),
		)
		if err != nil {
			return nil, nil, fmt.Errorf(
				"observability: stdout metric exporter: %w",
				err,
			)
		}

		reader = sdkmetric.NewPeriodicReader(
			exp,
			sdkmetric.WithInterval(cfg.MetricsInterval),
		)
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(reader),
		sdkmetric.WithResource(res),
	)

	return mp, promHandler, nil
}
