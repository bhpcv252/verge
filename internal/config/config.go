package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v10"
	"github.com/go-playground/validator/v10"
	"github.com/joho/godotenv"
)

type HTTP struct {
	Enabled bool `env:"ENABLED" envDefault:"true"`
	Port    int  `env:"PORT"    envDefault:"8080" validate:"gt=0,lt=65535"`
}

type GRPC struct {
	Enabled bool `env:"ENABLED" envDefault:"false"`
	Port    int  `env:"PORT"    envDefault:"9090"  validate:"gt=0,lt=65535"`
}

type Server struct {
	HTTP HTTP `envPrefix:"HTTP_"`
	GRPC GRPC `envPrefix:"GRPC_"`
}

type PGConfig struct {
	URL string `env:"URL" validate:"required,url"`
}

type RedisConfig struct {
	Enabled   bool          `env:"ENABLED"    envDefault:"false"`
	URL       string        `env:"URL"                           validate:"omitempty,url"`
	BranchTTL time.Duration `env:"BRANCH_TTL" envDefault:"30s"` // TTL for branch head cache entries
}

type OptionalDBConfig struct {
	Enabled bool   `env:"ENABLED" envDefault:"false"`
	URL     string `env:"URL"                        validate:"omitempty,url"`
}

type Storage struct {
	Postgres PGConfig         `envPrefix:"POSTGRES_"`
	Redis    RedisConfig      `envPrefix:"REDIS_"`
	Neo4j    OptionalDBConfig `envPrefix:"NEO4J_"`
}

type EventBusConfig struct {
	Enabled bool   `env:"ENABLED" envDefault:"false"`
	Type    string `env:"TYPE"    envDefault:"kafka"`
}

type OutboxConfig struct {
	// source type: "polling" (default) or "debezium"
	SourceType string `env:"SOURCE_TYPE" envDefault:"polling"`

	// polling source configuration (only used when SourceType=polling)
	PollInterval time.Duration `env:"POLL_INTERVAL" envDefault:"500ms"`

	// batch size for all source types
	BatchSize int `env:"BATCH_SIZE" envDefault:"100"`

	// debezium source configuration (only used when SourceType=debezium)
	DebeziumBrokers string `env:"DEBEZIUM_BROKERS"` // comma-separated, e.g. "kafka-1:9092,kafka-2:9092"
	DebeziumTopic   string `env:"DEBEZIUM_TOPIC"    envDefault:"verge.outbox.events"`
	DebeziumGroupID string `env:"DEBEZIUM_GROUP_ID" envDefault:"verge-worker"`

	// EventBus configuration (optional, for publishing to external consumers)
	EventBus EventBusConfig `envPrefix:"EVENTBUS_"`
}

// only read when Outbox.EventBus.Enabled = true and Outbox.EventBus.Type = "kafka"
type KafkaConfig struct {
	Brokers string `env:"BROKERS" envDefault:""` // comma-separated, e.g. "kafka:9092"
	Topic   string `env:"TOPIC"   envDefault:"verge.events"`
}

type OTelConfig struct {
	Enabled bool `env:"ENABLED" envDefault:"false"`

	Exporter string `env:"EXPORTER" envDefault:"stdout" validate:"omitempty,oneof=stdout otlp prometheus"`

	// only read when Exporter == "otlp" e.g., "otel-collector:4317"
	OTLPEndpoint string `env:"OTLP_ENDPOINT"`

	ServiceName string `env:"SERVICE_NAME" envDefault:"verge"`

	SampleRate float64 `env:"SAMPLE_RATE" envDefault:"1.0" validate:"min=0,max=1"`

	MetricsInterval time.Duration `env:"METRICS_INTERVAL" envDefault:"15s"`

	LogLevel string `env:"LOG_LEVEL" envDefault:"info" validate:"oneof=info debug"`
}

type Config struct {
	Server  Server       `envPrefix:"SERVER_"`
	Storage Storage      `envPrefix:"STORAGE_"`
	Outbox  OutboxConfig `envPrefix:"OUTBOX_"`
	Kafka   KafkaConfig  `envPrefix:"KAFKA_"`
	OTel    OTelConfig   `envPrefix:"OTEL_"`
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{}

	opts := env.Options{Prefix: "VERGE_"}
	if err := env.ParseWithOptions(cfg, opts); err != nil {
		return nil, fmt.Errorf("failed to parse environment variables: %w", err)
	}

	if err := validate(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func validate(cfg *Config) error {
	v := validator.New()

	v.RegisterStructValidation(validateServer, Server{})
	v.RegisterStructValidation(validateStorage, Storage{})
	v.RegisterStructValidation(validateOutbox, OutboxConfig{})
	v.RegisterStructValidation(validateOTel, OTelConfig{})

	return v.Struct(cfg)
}

func validateServer(sl validator.StructLevel) {
	s := sl.Current().Interface().(Server)

	if !s.HTTP.Enabled && !s.GRPC.Enabled {
		sl.ReportError(s.HTTP.Enabled, "HTTP", "http", "at-least-one-server", "")
		sl.ReportError(s.GRPC.Enabled, "GRPC", "grpc", "at-least-one-server", "")
	}
}

func validateStorage(sl validator.StructLevel) {
	s := sl.Current().Interface().(Storage)

	if s.Redis.Enabled && s.Redis.URL == "" {
		sl.ReportError(s.Redis.URL, "Redis.URL", "url", "required-if-enabled", "")
	}

	if s.Neo4j.Enabled && s.Neo4j.URL == "" {
		sl.ReportError(s.Neo4j.URL, "Neo4j.URL", "url", "required-if-enabled", "")
	}
}

func validateOutbox(sl validator.StructLevel) {
	o := sl.Current().Interface().(OutboxConfig)

	if o.SourceType != "polling" && o.SourceType != "debezium" {
		sl.ReportError(o.SourceType, "SourceType", "sourceType", "invalid-source-type", "")
	}

	if o.SourceType == "debezium" && o.DebeziumBrokers == "" {
		sl.ReportError(
			o.DebeziumBrokers,
			"DebeziumBrokers",
			"debeziumBrokers",
			"required-for-debezium",
			"",
		)
	}

	if o.EventBus.Enabled && o.EventBus.Type == "" {
		sl.ReportError(o.EventBus.Type, "EventBus.Type", "type", "required-if-enabled", "")
	}
}

func validateOTel(sl validator.StructLevel) {
	o := sl.Current().Interface().(OTelConfig)

	if o.Enabled && o.Exporter == "otlp" && o.OTLPEndpoint == "" {
		sl.ReportError(
			o.OTLPEndpoint,
			"OTLPEndpoint",
			"otlpEndpoint",
			"required-for-otlp",
			"",
		)
	}
}
