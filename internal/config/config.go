package config

import (
	"fmt"

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

type OptionalDBConfig struct {
	Enabled bool   `env:"ENABLED" envDefault:"false"`
	URL     string `env:"URL"                        validate:"omitempty,url"`
}

type Storage struct {
	Postgres PGConfig         `envPrefix:"POSTGRES_"`
	Redis    OptionalDBConfig `envPrefix:"REDIS_"`
	Neo4j    OptionalDBConfig `envPrefix:"NEO4J_"`
}

type Config struct {
	Server  Server  `envPrefix:"SERVER_"`
	Storage Storage `envPrefix:"STORAGE_"`
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
