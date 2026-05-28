package config

import (
	"os"
	"strings"
	"testing"
	"time"
)

// sets multiple environment variables and returns a cleanup function
func setEnv(t *testing.T, pairs map[string]string) {
	t.Helper()
	for k, v := range pairs {
		t.Setenv(k, v)
	}
}

// exhaustive list of all VERGE_ variables recognized by this service
// if you add a new env var to config.go, add it here as well with a test case
var knownVergeKeys = map[string]struct{}{
	// Server
	"VERGE_SERVER_HTTP_ENABLED": {},
	"VERGE_SERVER_HTTP_PORT":    {},
	"VERGE_SERVER_GRPC_ENABLED": {},
	"VERGE_SERVER_GRPC_PORT":    {},

	// Storage
	"VERGE_STORAGE_POSTGRES_URL": {},

	"VERGE_STORAGE_REDIS_ENABLED":    {},
	"VERGE_STORAGE_REDIS_URL":        {},
	"VERGE_STORAGE_REDIS_BRANCH_TTL": {},

	"VERGE_STORAGE_NEO4J_ENABLED": {},
	"VERGE_STORAGE_NEO4J_URL":     {},

	// Outbox worker
	"VERGE_OUTBOX_SOURCE_TYPE":       {},
	"VERGE_OUTBOX_POLL_INTERVAL":     {},
	"VERGE_OUTBOX_BATCH_SIZE":        {},
	"VERGE_OUTBOX_EVENTBUS_ENABLED":  {},
	"VERGE_OUTBOX_EVENTBUS_TYPE":     {},
	"VERGE_OUTBOX_DEBEZIUM_BROKERS":  {},
	"VERGE_OUTBOX_DEBEZIUM_TOPIC":    {},
	"VERGE_OUTBOX_DEBEZIUM_GROUP_ID": {},

	"VERGE_KAFKA_BROKERS": {},
	"VERGE_KAFKA_TOPIC":   {},

	// OpenTelemetry observability
	"VERGE_OTEL_ENABLED":          {},
	"VERGE_OTEL_EXPORTER":         {},
	"VERGE_OTEL_OTLP_ENDPOINT":    {},
	"VERGE_OTEL_SERVICE_NAME":     {},
	"VERGE_OTEL_SAMPLE_RATE":      {},
	"VERGE_OTEL_METRICS_INTERVAL": {},
	"VERGE_OTEL_LOG_LEVEL":        {},

	// Auth
	"VERGE_AUTH_ENABLED": {},
	"VERGE_AUTH_KEYS":    {},
}

// NOTE: this uses os.Unsetenv (not t.Setenv) intentionally, we want a hard
// reset before setting only the vars a test cares about via setEnv/t.Setenv
func clearVergeEnv(t *testing.T) {
	t.Helper()
	for k := range knownVergeKeys {
		os.Unsetenv(k)
	}
}

func baseEnv() map[string]string {
	return map[string]string{
		// server
		"VERGE_SERVER_HTTP_ENABLED": "true",
		"VERGE_SERVER_HTTP_PORT":    "8080",
		"VERGE_SERVER_GRPC_ENABLED": "false",
		"VERGE_SERVER_GRPC_PORT":    "9090",

		// storage
		"VERGE_STORAGE_POSTGRES_URL":  "postgres://verge:changeme@postgres:5432/verge?sslmode=disable",
		"VERGE_STORAGE_REDIS_ENABLED": "false",
		"VERGE_STORAGE_REDIS_URL":     "",
		"VERGE_STORAGE_NEO4J_ENABLED": "false",
		"VERGE_STORAGE_NEO4J_URL":     "",
	}
}

func TestNoUnrecognisedVergeEnvVars(t *testing.T) {
	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		if !strings.HasPrefix(key, "VERGE_") {
			continue
		}
		if _, known := knownVergeKeys[key]; !known {
			t.Errorf(
				"unrecognised VERGE_ env var %q - either add it to config.go and knownVergeKeys, or remove it from the environment",
				key,
			)
		}
	}
}

// default values

func TestLoad_ServerDefaults(t *testing.T) {
	clearVergeEnv(t)
	setEnv(t, map[string]string{
		"VERGE_STORAGE_POSTGRES_URL": "postgres://verge:changeme@localhost:5432/verge?sslmode=disable",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if !cfg.Server.HTTP.Enabled {
		t.Error("Server.HTTP.Enabled default: want true, got false")
	}
	if cfg.Server.HTTP.Port != 8080 {
		t.Errorf("Server.HTTP.Port default: want 8080, got %d", cfg.Server.HTTP.Port)
	}
	if cfg.Server.GRPC.Enabled {
		t.Error("Server.GRPC.Enabled default: want false, got true")
	}
	if cfg.Server.GRPC.Port != 9090 {
		t.Errorf("Server.GRPC.Port default: want 9090, got %d", cfg.Server.GRPC.Port)
	}
}

func TestLoad_StorageDefaults(t *testing.T) {
	clearVergeEnv(t)
	setEnv(t, map[string]string{
		"VERGE_STORAGE_POSTGRES_URL": "postgres://verge:changeme@localhost:5432/verge?sslmode=disable",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if cfg.Storage.Redis.Enabled {
		t.Error("Storage.Redis.Enabled default: want false, got true")
	}
	if cfg.Storage.Redis.BranchTTL != 30*time.Second {
		t.Errorf("Storage.Redis.BranchTTL default: want 30s, got %s", cfg.Storage.Redis.BranchTTL)
	}
	if cfg.Storage.Neo4j.Enabled {
		t.Error("Storage.Neo4j.Enabled default: want false, got true")
	}
}

func TestLoad_OutboxDefaults(t *testing.T) {
	clearVergeEnv(t)
	setEnv(t, map[string]string{
		"VERGE_STORAGE_POSTGRES_URL": "postgres://verge:changeme@localhost:5432/verge?sslmode=disable",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if cfg.Outbox.SourceType != "polling" {
		t.Errorf("Outbox.SourceType default: want %q, got %q", "polling", cfg.Outbox.SourceType)
	}
	if cfg.Outbox.PollInterval != 500*time.Millisecond {
		t.Errorf("Outbox.PollInterval default: want 500ms, got %s", cfg.Outbox.PollInterval)
	}
	if cfg.Outbox.BatchSize != 100 {
		t.Errorf("Outbox.BatchSize default: want 100, got %d", cfg.Outbox.BatchSize)
	}
	if cfg.Outbox.DebeziumTopic != "verge.outbox.events" {
		t.Errorf(
			"Outbox.DebeziumTopic default: want %q, got %q",
			"verge.outbox.events",
			cfg.Outbox.DebeziumTopic,
		)
	}
	if cfg.Outbox.DebeziumGroupID != "verge-worker" {
		t.Errorf(
			"Outbox.DebeziumGroupID default: want %q, got %q",
			"verge-worker",
			cfg.Outbox.DebeziumGroupID,
		)
	}
	if cfg.Outbox.DebeziumBrokers != "" {
		t.Errorf("Outbox.DebeziumBrokers default: want empty, got %q", cfg.Outbox.DebeziumBrokers)
	}
	if cfg.Outbox.EventBus.Enabled {
		t.Error("Outbox.EventBus.Enabled default: want false, got true")
	}
	if cfg.Outbox.EventBus.Type != "kafka" {
		t.Errorf("Outbox.EventBus.Type default: want %q, got %q", "kafka", cfg.Outbox.EventBus.Type)
	}
}

func TestLoad_KafkaDefaults(t *testing.T) {
	clearVergeEnv(t)
	setEnv(t, map[string]string{
		"VERGE_STORAGE_POSTGRES_URL": "postgres://verge:changeme@localhost:5432/verge?sslmode=disable",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if cfg.Kafka.Topic != "verge.events" {
		t.Errorf("Kafka.Topic default: want \"verge.events\", got %q", cfg.Kafka.Topic)
	}
	if cfg.Kafka.Brokers != "" {
		t.Errorf("Kafka.Brokers default: want empty string, got %q", cfg.Kafka.Brokers)
	}
}

func TestLoad_OTelDefaults(t *testing.T) {
	clearVergeEnv(t)
	setEnv(t, map[string]string{
		"VERGE_STORAGE_POSTGRES_URL": "postgres://verge:changeme@localhost:5432/verge?sslmode=disable",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if cfg.OTel.Enabled {
		t.Error("OTel.Enabled default: want false, got true")
	}
	if cfg.OTel.Exporter != "stdout" {
		t.Errorf("OTel.Exporter default: want %q, got %q", "stdout", cfg.OTel.Exporter)
	}
	if cfg.OTel.OTLPEndpoint != "" {
		t.Errorf("OTel.OTLPEndpoint default: want empty, got %q", cfg.OTel.OTLPEndpoint)
	}
	if cfg.OTel.ServiceName != "verge" {
		t.Errorf("OTel.ServiceName default: want %q, got %q", "verge", cfg.OTel.ServiceName)
	}
	if cfg.OTel.SampleRate != 1.0 {
		t.Errorf("OTel.SampleRate default: want 1.0, got %f", cfg.OTel.SampleRate)
	}
	if cfg.OTel.MetricsInterval != 15*time.Second {
		t.Errorf("OTel.MetricsInterval default: want 15s, got %s", cfg.OTel.MetricsInterval)
	}
	if cfg.OTel.LogLevel != "info" {
		t.Errorf("OTel.LogLevel default: want %q, got %q", "info", cfg.OTel.LogLevel)
	}
}

// server config

func TestLoad_OnlyHTTPEnabled(t *testing.T) {
	clearVergeEnv(t)
	setEnv(t, map[string]string{
		"VERGE_SERVER_HTTP_ENABLED":  "true",
		"VERGE_SERVER_GRPC_ENABLED":  "false",
		"VERGE_STORAGE_POSTGRES_URL": "postgres://u:p@localhost:5432/db?sslmode=disable",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error with only HTTP enabled, got: %v", err)
	}
	if !cfg.Server.HTTP.Enabled {
		t.Error("expected HTTP to be enabled")
	}
	if cfg.Server.GRPC.Enabled {
		t.Error("expected GRPC to be disabled")
	}
}

func TestLoad_OnlyGRPCEnabled(t *testing.T) {
	clearVergeEnv(t)
	setEnv(t, map[string]string{
		"VERGE_SERVER_HTTP_ENABLED":  "false",
		"VERGE_SERVER_GRPC_ENABLED":  "true",
		"VERGE_STORAGE_POSTGRES_URL": "postgres://u:p@localhost:5432/db?sslmode=disable",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error with only GRPC enabled, got: %v", err)
	}
	if cfg.Server.HTTP.Enabled {
		t.Error("expected HTTP to be disabled")
	}
	if !cfg.Server.GRPC.Enabled {
		t.Error("expected GRPC to be enabled")
	}
}

func TestLoad_BothServersEnabled(t *testing.T) {
	clearVergeEnv(t)
	setEnv(t, map[string]string{
		"VERGE_SERVER_HTTP_ENABLED":  "true",
		"VERGE_SERVER_GRPC_ENABLED":  "true",
		"VERGE_STORAGE_POSTGRES_URL": "postgres://u:p@localhost:5432/db?sslmode=disable",
	})

	_, err := Load()
	if err != nil {
		t.Fatalf("expected no error with both servers enabled, got: %v", err)
	}
}

func TestLoad_BothServersDisabled(t *testing.T) {
	clearVergeEnv(t)
	setEnv(t, map[string]string{
		"VERGE_SERVER_HTTP_ENABLED":  "false",
		"VERGE_SERVER_GRPC_ENABLED":  "false",
		"VERGE_STORAGE_POSTGRES_URL": "postgres://u:p@localhost:5432/db?sslmode=disable",
	})

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when both servers are disabled, got nil")
	}
}

func TestLoad_HTTPPort_Zero_IsInvalid(t *testing.T) {
	clearVergeEnv(t)
	e := baseEnv()
	e["VERGE_SERVER_HTTP_PORT"] = "0"
	setEnv(t, e)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for HTTP port 0 (validate: gt=0)")
	}
}

func TestLoad_HTTPPort_65535_IsInvalid(t *testing.T) {
	clearVergeEnv(t)
	e := baseEnv()
	e["VERGE_SERVER_HTTP_PORT"] = "65535"
	setEnv(t, e)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for HTTP port 65535 (validate: lt=65535)")
	}
}

func TestLoad_HTTPPort_ValidBoundaries(t *testing.T) {
	// gt=0, lt=65535 → valid range is [1, 65534]
	for _, port := range []string{"1", "65534"} {
		port := port
		t.Run("port="+port, func(t *testing.T) {
			clearVergeEnv(t)
			e := baseEnv()
			e["VERGE_SERVER_HTTP_PORT"] = port
			setEnv(t, e)

			_, err := Load()
			if err != nil {
				t.Fatalf("port %s should be valid, got: %v", port, err)
			}
		})
	}
}

func TestLoad_GRPCPort_Zero_IsInvalid(t *testing.T) {
	clearVergeEnv(t)
	e := baseEnv()
	e["VERGE_SERVER_GRPC_ENABLED"] = "true"
	e["VERGE_SERVER_GRPC_PORT"] = "0"
	setEnv(t, e)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for GRPC port 0 (validate: gt=0)")
	}
}

func TestLoad_GRPCPort_65535_IsInvalid(t *testing.T) {
	clearVergeEnv(t)
	e := baseEnv()
	e["VERGE_SERVER_GRPC_ENABLED"] = "true"
	e["VERGE_SERVER_GRPC_PORT"] = "65535"
	setEnv(t, e)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for GRPC port 65535 (validate: lt=65535)")
	}
}

func TestLoad_GRPCPort_ValidBoundaries(t *testing.T) {
	for _, port := range []string{"1", "65534"} {
		port := port
		t.Run("port="+port, func(t *testing.T) {
			clearVergeEnv(t)
			e := baseEnv()
			e["VERGE_SERVER_GRPC_ENABLED"] = "true"
			e["VERGE_SERVER_GRPC_PORT"] = port
			setEnv(t, e)

			_, err := Load()
			if err != nil {
				t.Fatalf("GRPC port %s should be valid, got: %v", port, err)
			}
		})
	}
}

// postgres config

func TestLoad_MissingPostgresURL(t *testing.T) {
	clearVergeEnv(t)
	setEnv(t, map[string]string{
		"VERGE_SERVER_HTTP_ENABLED": "true",
		// VERGE_STORAGE_POSTGRES_URL deliberately omitted
	})

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when Postgres URL is missing")
	}
}

func TestLoad_InvalidPostgresURL(t *testing.T) {
	clearVergeEnv(t)
	e := baseEnv()
	e["VERGE_STORAGE_POSTGRES_URL"] = "not-a-valid-url"
	setEnv(t, e)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid Postgres URL")
	}
}

func TestLoad_ValidPostgresURL(t *testing.T) {
	clearVergeEnv(t)
	const pgURL = "postgres://u:p@localhost:5432/mydb?sslmode=disable"
	setEnv(t, map[string]string{
		"VERGE_SERVER_HTTP_ENABLED":  "true",
		"VERGE_STORAGE_POSTGRES_URL": pgURL,
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if cfg.Storage.Postgres.URL != pgURL {
		t.Errorf("Storage.Postgres.URL: want %q, got %q", pgURL, cfg.Storage.Postgres.URL)
	}
}

// Redis config

func TestLoad_RedisEnabledWithURL(t *testing.T) {
	clearVergeEnv(t)
	e := baseEnv()
	e["VERGE_STORAGE_REDIS_ENABLED"] = "true"
	e["VERGE_STORAGE_REDIS_URL"] = "redis://localhost:6379"
	setEnv(t, e)

	_, err := Load()
	if err != nil {
		t.Fatalf("expected no error with Redis enabled and URL set, got: %v", err)
	}
}

func TestLoad_RedisEnabledWithoutURL(t *testing.T) {
	clearVergeEnv(t)
	e := baseEnv()
	e["VERGE_STORAGE_REDIS_ENABLED"] = "true"
	e["VERGE_STORAGE_REDIS_URL"] = ""
	setEnv(t, e)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when Redis is enabled but URL is empty")
	}
}

func TestLoad_RedisEnabledWithInvalidURL(t *testing.T) {
	clearVergeEnv(t)
	e := baseEnv()
	e["VERGE_STORAGE_REDIS_ENABLED"] = "true"
	e["VERGE_STORAGE_REDIS_URL"] = "not-a-url"
	setEnv(t, e)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid Redis URL")
	}
}

func TestLoad_RedisDisabledWithoutURL(t *testing.T) {
	clearVergeEnv(t)
	e := baseEnv()
	e["VERGE_STORAGE_REDIS_ENABLED"] = "false"
	e["VERGE_STORAGE_REDIS_URL"] = ""
	setEnv(t, e)

	_, err := Load()
	if err != nil {
		t.Fatalf("expected no error when Redis is disabled and URL is empty, got: %v", err)
	}
}

func TestLoad_RedisDisabledURLPresentIsAllowed(t *testing.T) {
	clearVergeEnv(t)
	e := baseEnv()
	e["VERGE_STORAGE_REDIS_ENABLED"] = "false"
	e["VERGE_STORAGE_REDIS_URL"] = "redis://localhost:6379"
	setEnv(t, e)

	_, err := Load()
	if err != nil {
		t.Fatalf("expected no error when Redis is disabled with a URL present, got: %v", err)
	}
}

func TestLoad_RedisBranchTTL_CustomValue(t *testing.T) {
	clearVergeEnv(t)
	e := baseEnv()
	e["VERGE_STORAGE_REDIS_ENABLED"] = "true"
	e["VERGE_STORAGE_REDIS_URL"] = "redis://localhost:6379"
	e["VERGE_STORAGE_REDIS_BRANCH_TTL"] = "2m"
	setEnv(t, e)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if cfg.Storage.Redis.BranchTTL != 2*time.Minute {
		t.Errorf("Storage.Redis.BranchTTL: want 2m, got %s", cfg.Storage.Redis.BranchTTL)
	}
}

func TestLoad_RedisBranchTTL_InvalidDuration(t *testing.T) {
	clearVergeEnv(t)
	e := baseEnv()
	e["VERGE_STORAGE_REDIS_BRANCH_TTL"] = "not-a-duration"
	setEnv(t, e)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid BranchTTL duration string")
	}
}

// Neo4j config

func TestLoad_Neo4jEnabledWithURL(t *testing.T) {
	clearVergeEnv(t)
	e := baseEnv()
	e["VERGE_STORAGE_NEO4J_ENABLED"] = "true"
	e["VERGE_STORAGE_NEO4J_URL"] = "bolt://localhost:7687"
	setEnv(t, e)

	_, err := Load()
	if err != nil {
		t.Fatalf("expected no error with Neo4j enabled and URL set, got: %v", err)
	}
}

func TestLoad_Neo4jEnabledWithoutURL(t *testing.T) {
	clearVergeEnv(t)
	e := baseEnv()
	e["VERGE_STORAGE_NEO4J_ENABLED"] = "true"
	e["VERGE_STORAGE_NEO4J_URL"] = ""
	setEnv(t, e)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when Neo4j is enabled but URL is empty")
	}
}

func TestLoad_Neo4jEnabledWithInvalidURL(t *testing.T) {
	clearVergeEnv(t)
	e := baseEnv()
	e["VERGE_STORAGE_NEO4J_ENABLED"] = "true"
	e["VERGE_STORAGE_NEO4J_URL"] = "not-a-url"
	setEnv(t, e)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid Neo4j URL")
	}
}

func TestLoad_Neo4jDisabledWithoutURL(t *testing.T) {
	clearVergeEnv(t)
	e := baseEnv()
	e["VERGE_STORAGE_NEO4J_ENABLED"] = "false"
	e["VERGE_STORAGE_NEO4J_URL"] = ""
	setEnv(t, e)

	_, err := Load()
	if err != nil {
		t.Fatalf("expected no error when Neo4j is disabled and URL is empty, got: %v", err)
	}
}

func TestLoad_Neo4jDisabledURLPresentIsAllowed(t *testing.T) {
	clearVergeEnv(t)
	e := baseEnv()
	e["VERGE_STORAGE_NEO4J_ENABLED"] = "false"
	e["VERGE_STORAGE_NEO4J_URL"] = "bolt://localhost:7687"
	setEnv(t, e)

	_, err := Load()
	if err != nil {
		t.Fatalf("expected no error when Neo4j is disabled with a URL present, got: %v", err)
	}
}

// outbox config

func TestLoad_OutboxCustomPollInterval(t *testing.T) {
	clearVergeEnv(t)
	e := baseEnv()
	e["VERGE_OUTBOX_POLL_INTERVAL"] = "1s"
	setEnv(t, e)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if cfg.Outbox.PollInterval != time.Second {
		t.Errorf("Outbox.PollInterval: want 1s, got %s", cfg.Outbox.PollInterval)
	}
}

func TestLoad_OutboxInvalidPollInterval(t *testing.T) {
	clearVergeEnv(t)
	e := baseEnv()
	e["VERGE_OUTBOX_POLL_INTERVAL"] = "not-a-duration"
	setEnv(t, e)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid PollInterval duration string")
	}
}

func TestLoad_OutboxCustomBatchSize(t *testing.T) {
	clearVergeEnv(t)
	e := baseEnv()
	e["VERGE_OUTBOX_BATCH_SIZE"] = "50"
	setEnv(t, e)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if cfg.Outbox.BatchSize != 50 {
		t.Errorf("Outbox.BatchSize: want 50, got %d", cfg.Outbox.BatchSize)
	}
}

func TestLoad_OutboxInvalidBatchSize(t *testing.T) {
	clearVergeEnv(t)
	e := baseEnv()
	e["VERGE_OUTBOX_BATCH_SIZE"] = "not-a-number"
	setEnv(t, e)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for non-integer BatchSize")
	}
}

// outbox source type

func TestLoad_OutboxSourceType_DefaultIsPolling(t *testing.T) {
	clearVergeEnv(t)
	setEnv(t, map[string]string{
		"VERGE_STORAGE_POSTGRES_URL": "postgres://u:p@localhost:5432/db?sslmode=disable",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if cfg.Outbox.SourceType != "polling" {
		t.Errorf("Outbox.SourceType default: want %q, got %q", "polling", cfg.Outbox.SourceType)
	}
}

func TestLoad_OutboxSourceType_Polling_IsValid(t *testing.T) {
	clearVergeEnv(t)
	e := baseEnv()
	e["VERGE_OUTBOX_SOURCE_TYPE"] = "polling"
	setEnv(t, e)

	_, err := Load()
	if err != nil {
		t.Fatalf("source_type=polling should be valid, got: %v", err)
	}
}

func TestLoad_OutboxSourceType_Debezium_WithBrokers_IsValid(t *testing.T) {
	clearVergeEnv(t)
	e := baseEnv()
	e["VERGE_OUTBOX_SOURCE_TYPE"] = "debezium"
	e["VERGE_OUTBOX_DEBEZIUM_BROKERS"] = "kafka:9092"
	setEnv(t, e)

	_, err := Load()
	if err != nil {
		t.Fatalf("source_type=debezium with brokers set should be valid, got: %v", err)
	}
}

func TestLoad_OutboxSourceType_Debezium_WithoutBrokers_IsInvalid(t *testing.T) {
	clearVergeEnv(t)
	e := baseEnv()
	e["VERGE_OUTBOX_SOURCE_TYPE"] = "debezium"
	// VERGE_OUTBOX_DEBEZIUM_BROKERS deliberately omitted
	setEnv(t, e)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when source_type=debezium but DEBEZIUM_BROKERS is empty")
	}
}

func TestLoad_OutboxSourceType_Invalid_IsRejected(t *testing.T) {
	for _, invalid := range []string{"kafka", "rabbitmq", "cdc", "none", "POLLING"} {
		invalid := invalid
		t.Run("type="+invalid, func(t *testing.T) {
			clearVergeEnv(t)
			e := baseEnv()
			e["VERGE_OUTBOX_SOURCE_TYPE"] = invalid
			setEnv(t, e)

			_, err := Load()
			if err == nil {
				t.Fatalf("source_type=%q should be invalid, expected an error", invalid)
			}
		})
	}
}

// debezium config

func TestLoad_OutboxDebezium_CustomBrokers(t *testing.T) {
	clearVergeEnv(t)
	e := baseEnv()
	e["VERGE_OUTBOX_SOURCE_TYPE"] = "debezium"
	e["VERGE_OUTBOX_DEBEZIUM_BROKERS"] = "kafka-1:9092,kafka-2:9092"
	setEnv(t, e)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if cfg.Outbox.DebeziumBrokers != "kafka-1:9092,kafka-2:9092" {
		t.Errorf("Outbox.DebeziumBrokers: want %q, got %q",
			"kafka-1:9092,kafka-2:9092", cfg.Outbox.DebeziumBrokers)
	}
}

func TestLoad_OutboxDebezium_CustomTopic(t *testing.T) {
	clearVergeEnv(t)
	e := baseEnv()
	e["VERGE_OUTBOX_SOURCE_TYPE"] = "debezium"
	e["VERGE_OUTBOX_DEBEZIUM_BROKERS"] = "kafka:9092"
	e["VERGE_OUTBOX_DEBEZIUM_TOPIC"] = "my.custom.topic"
	setEnv(t, e)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if cfg.Outbox.DebeziumTopic != "my.custom.topic" {
		t.Errorf(
			"Outbox.DebeziumTopic: want %q, got %q",
			"my.custom.topic",
			cfg.Outbox.DebeziumTopic,
		)
	}
}

func TestLoad_OutboxDebezium_CustomGroupID(t *testing.T) {
	clearVergeEnv(t)
	e := baseEnv()
	e["VERGE_OUTBOX_SOURCE_TYPE"] = "debezium"
	e["VERGE_OUTBOX_DEBEZIUM_BROKERS"] = "kafka:9092"
	e["VERGE_OUTBOX_DEBEZIUM_GROUP_ID"] = "my-consumer-group"
	setEnv(t, e)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if cfg.Outbox.DebeziumGroupID != "my-consumer-group" {
		t.Errorf("Outbox.DebeziumGroupID: want %q, got %q",
			"my-consumer-group", cfg.Outbox.DebeziumGroupID)
	}
}

// outbox eventbus config

func TestLoad_OutboxEventBus_EnabledKafka_IsValid(t *testing.T) {
	clearVergeEnv(t)
	e := baseEnv()
	e["VERGE_OUTBOX_EVENTBUS_ENABLED"] = "true"
	e["VERGE_OUTBOX_EVENTBUS_TYPE"] = "kafka"
	setEnv(t, e)

	_, err := Load()
	if err != nil {
		t.Fatalf("expected no error with EventBus enabled and type=kafka, got: %v", err)
	}
}

func TestLoad_OutboxEventBus_EnabledCustomType_IsAllowed(t *testing.T) {
	for _, busType := range []string{"kafka", "rabbitmq", "sqs", "pubsub", "custom"} {
		busType := busType
		t.Run("type="+busType, func(t *testing.T) {
			clearVergeEnv(t)
			e := baseEnv()
			e["VERGE_OUTBOX_EVENTBUS_ENABLED"] = "true"
			e["VERGE_OUTBOX_EVENTBUS_TYPE"] = busType
			setEnv(t, e)

			_, err := Load()
			if err != nil {
				t.Fatalf("type %q should be valid when EventBus is enabled, got: %v", busType, err)
			}
		})
	}
}

func TestLoad_OutboxEventBus_EnabledEmptyType_IsInvalid(t *testing.T) {
	cfg := &Config{
		Server: Server{
			HTTP: HTTP{Enabled: true, Port: 8080},
			GRPC: GRPC{Enabled: false, Port: 9090},
		},
		Storage: Storage{
			Postgres: PGConfig{URL: "postgres://u:p@localhost:5432/db?sslmode=disable"},
		},
		Outbox: OutboxConfig{
			SourceType:   "polling",
			PollInterval: 500 * time.Millisecond,
			BatchSize:    100,
			EventBus: EventBusConfig{
				Enabled: true,
				Type:    "", // explicitly empty
			},
		},
		OTel: OTelConfig{
			Exporter:        "stdout",
			ServiceName:     "verge",
			SampleRate:      1.0,
			MetricsInterval: 15 * time.Second,
			LogLevel:        "info",
		},
	}

	if err := validate(cfg); err == nil {
		t.Fatal("expected error when EventBus is enabled with an empty type")
	}
}

func TestLoad_OutboxEventBus_DisabledTypeIgnored(t *testing.T) {
	for _, busType := range []string{"", "rabbitmq", "sqs"} {
		busType := busType
		t.Run("type="+busType, func(t *testing.T) {
			clearVergeEnv(t)
			e := baseEnv()
			e["VERGE_OUTBOX_EVENTBUS_ENABLED"] = "false"
			e["VERGE_OUTBOX_EVENTBUS_TYPE"] = busType
			setEnv(t, e)

			_, err := Load()
			if err != nil {
				t.Fatalf(
					"type %q should be allowed when EventBus is disabled, got: %v",
					busType,
					err,
				)
			}
		})
	}
}

// kafka config

func TestLoad_KafkaCustomValues(t *testing.T) {
	clearVergeEnv(t)
	e := baseEnv()
	e["VERGE_OUTBOX_EVENTBUS_ENABLED"] = "true"
	e["VERGE_OUTBOX_EVENTBUS_TYPE"] = "kafka"
	e["VERGE_KAFKA_BROKERS"] = "kafka1:9092,kafka2:9092"
	e["VERGE_KAFKA_TOPIC"] = "my-custom-topic"
	setEnv(t, e)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if cfg.Kafka.Brokers != "kafka1:9092,kafka2:9092" {
		t.Errorf("Kafka.Brokers: want %q, got %q", "kafka1:9092,kafka2:9092", cfg.Kafka.Brokers)
	}
	if cfg.Kafka.Topic != "my-custom-topic" {
		t.Errorf("Kafka.Topic: want %q, got %q", "my-custom-topic", cfg.Kafka.Topic)
	}
}

// OTel config

func TestLoad_OTel_Disabled_OtherFieldsIgnored(t *testing.T) {
	clearVergeEnv(t)
	e := baseEnv()
	e["VERGE_OTEL_ENABLED"] = "false"
	// OTLP endpoint deliberately absent even though exporter=otlp would normally require it
	e["VERGE_OTEL_EXPORTER"] = "otlp"
	setEnv(t, e)

	_, err := Load()
	if err != nil {
		t.Fatalf("expected no error when OTel is disabled, got: %v", err)
	}
}

func TestLoad_OTel_Enabled_StdoutExporter_IsValid(t *testing.T) {
	clearVergeEnv(t)
	e := baseEnv()
	e["VERGE_OTEL_ENABLED"] = "true"
	e["VERGE_OTEL_EXPORTER"] = "stdout"
	setEnv(t, e)

	_, err := Load()
	if err != nil {
		t.Fatalf("exporter=stdout should be valid, got: %v", err)
	}
}

func TestLoad_OTel_Enabled_PrometheusExporter_IsValid(t *testing.T) {
	clearVergeEnv(t)
	e := baseEnv()
	e["VERGE_OTEL_ENABLED"] = "true"
	e["VERGE_OTEL_EXPORTER"] = "prometheus"
	setEnv(t, e)

	_, err := Load()
	if err != nil {
		t.Fatalf("exporter=prometheus should be valid, got: %v", err)
	}
}

func TestLoad_OTel_Enabled_OTLPExporter_WithEndpoint_IsValid(t *testing.T) {
	clearVergeEnv(t)
	e := baseEnv()
	e["VERGE_OTEL_ENABLED"] = "true"
	e["VERGE_OTEL_EXPORTER"] = "otlp"
	e["VERGE_OTEL_OTLP_ENDPOINT"] = "otel-collector:4317"
	setEnv(t, e)

	_, err := Load()
	if err != nil {
		t.Fatalf("exporter=otlp with endpoint set should be valid, got: %v", err)
	}
}

func TestLoad_OTel_Enabled_OTLPExporter_WithoutEndpoint_IsInvalid(t *testing.T) {
	clearVergeEnv(t)
	e := baseEnv()
	e["VERGE_OTEL_ENABLED"] = "true"
	e["VERGE_OTEL_EXPORTER"] = "otlp"
	// VERGE_OTEL_OTLP_ENDPOINT deliberately omitted
	setEnv(t, e)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when exporter=otlp but OTLP_ENDPOINT is empty")
	}
}

func TestLoad_OTel_InvalidExporter_IsRejected(t *testing.T) {
	for _, exporter := range []string{"jaeger", "zipkin", "datadog", "STDOUT"} {
		exporter := exporter
		t.Run("exporter="+exporter, func(t *testing.T) {
			clearVergeEnv(t)
			e := baseEnv()
			e["VERGE_OTEL_ENABLED"] = "true"
			e["VERGE_OTEL_EXPORTER"] = exporter
			setEnv(t, e)

			_, err := Load()
			if err == nil {
				t.Fatalf("exporter=%q should be invalid, expected an error", exporter)
			}
		})
	}
}

func TestLoad_OTel_SampleRate_ValidBoundaries(t *testing.T) {
	for _, rate := range []string{"0", "0.0", "0.1", "0.5", "1", "1.0"} {
		rate := rate
		t.Run("rate="+rate, func(t *testing.T) {
			clearVergeEnv(t)
			e := baseEnv()
			e["VERGE_OTEL_ENABLED"] = "true"
			e["VERGE_OTEL_EXPORTER"] = "stdout"
			e["VERGE_OTEL_SAMPLE_RATE"] = rate
			setEnv(t, e)

			_, err := Load()
			if err != nil {
				t.Fatalf("sample_rate=%s should be valid, got: %v", rate, err)
			}
		})
	}
}

func TestLoad_OTel_SampleRate_OutOfRange_IsInvalid(t *testing.T) {
	for _, rate := range []string{"-0.1", "1.1", "2.0", "-1"} {
		rate := rate
		t.Run("rate="+rate, func(t *testing.T) {
			clearVergeEnv(t)
			e := baseEnv()
			e["VERGE_OTEL_ENABLED"] = "true"
			e["VERGE_OTEL_EXPORTER"] = "stdout"
			e["VERGE_OTEL_SAMPLE_RATE"] = rate
			setEnv(t, e)

			_, err := Load()
			if err == nil {
				t.Fatalf(
					"sample_rate=%s should be invalid (validate: min=0,max=1), got no error",
					rate,
				)
			}
		})
	}
}

func TestLoad_OTel_LogLevel_Debug_IsValid(t *testing.T) {
	clearVergeEnv(t)
	e := baseEnv()
	e["VERGE_OTEL_LOG_LEVEL"] = "debug"
	setEnv(t, e)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("log_level=debug should be valid, got: %v", err)
	}
	if cfg.OTel.LogLevel != "debug" {
		t.Errorf("OTel.LogLevel: want %q, got %q", "debug", cfg.OTel.LogLevel)
	}
}

func TestLoad_OTel_LogLevel_Invalid_IsRejected(t *testing.T) {
	for _, level := range []string{"warn", "error", "trace", "INFO", "DEBUG"} {
		level := level
		t.Run("level="+level, func(t *testing.T) {
			clearVergeEnv(t)
			e := baseEnv()
			e["VERGE_OTEL_LOG_LEVEL"] = level
			setEnv(t, e)

			_, err := Load()
			if err == nil {
				t.Fatalf("log_level=%q should be invalid (oneof=info debug), got no error", level)
			}
		})
	}
}

func TestLoad_OTel_CustomServiceName(t *testing.T) {
	clearVergeEnv(t)
	e := baseEnv()
	e["VERGE_OTEL_SERVICE_NAME"] = "verge-staging"
	setEnv(t, e)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if cfg.OTel.ServiceName != "verge-staging" {
		t.Errorf("OTel.ServiceName: want %q, got %q", "verge-staging", cfg.OTel.ServiceName)
	}
}

func TestLoad_OTel_CustomMetricsInterval(t *testing.T) {
	clearVergeEnv(t)
	e := baseEnv()
	e["VERGE_OTEL_METRICS_INTERVAL"] = "30s"
	setEnv(t, e)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if cfg.OTel.MetricsInterval != 30*time.Second {
		t.Errorf("OTel.MetricsInterval: want 30s, got %s", cfg.OTel.MetricsInterval)
	}
}

func TestLoad_OTel_InvalidMetricsInterval(t *testing.T) {
	clearVergeEnv(t)
	e := baseEnv()
	e["VERGE_OTEL_METRICS_INTERVAL"] = "not-a-duration"
	setEnv(t, e)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid MetricsInterval duration string")
	}
}

func TestLoad_OTel_OTLPEndpoint_StoredVerbatim(t *testing.T) {
	clearVergeEnv(t)
	e := baseEnv()
	e["VERGE_OTEL_ENABLED"] = "true"
	e["VERGE_OTEL_EXPORTER"] = "otlp"
	e["VERGE_OTEL_OTLP_ENDPOINT"] = "otel-collector:4317"
	setEnv(t, e)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if cfg.OTel.OTLPEndpoint != "otel-collector:4317" {
		t.Errorf("OTel.OTLPEndpoint: want %q, got %q", "otel-collector:4317", cfg.OTel.OTLPEndpoint)
	}
}

// auth config

func TestLoad_AuthDefaults(t *testing.T) {
	clearVergeEnv(t)
	setEnv(t, map[string]string{
		"VERGE_STORAGE_POSTGRES_URL": "postgres://verge:changeme@localhost:5432/verge?sslmode=disable",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if cfg.Auth.Enabled {
		t.Error("Auth.Enabled default: want false, got true")
	}
	if len(cfg.Auth.Keys) != 0 {
		t.Errorf("Auth.Keys default: want empty slice, got %v", cfg.Auth.Keys)
	}
}

func TestLoad_Auth_Disabled_NoKeysRequired(t *testing.T) {
	clearVergeEnv(t)
	e := baseEnv()
	e["VERGE_AUTH_ENABLED"] = "false"
	// VERGE_AUTH_KEYS deliberately omitted
	setEnv(t, e)

	_, err := Load()
	if err != nil {
		t.Fatalf("expected no error when auth is disabled with no keys, got: %v", err)
	}
}

func TestLoad_Auth_Disabled_KeysPresentIsAllowed(t *testing.T) {
	clearVergeEnv(t)
	e := baseEnv()
	e["VERGE_AUTH_ENABLED"] = "false"
	e["VERGE_AUTH_KEYS"] = "key-abc123"
	setEnv(t, e)

	_, err := Load()
	if err != nil {
		t.Fatalf("expected no error when auth is disabled with keys present, got: %v", err)
	}
}

func TestLoad_Auth_Enabled_WithSingleKey_IsValid(t *testing.T) {
	clearVergeEnv(t)
	e := baseEnv()
	e["VERGE_AUTH_ENABLED"] = "true"
	e["VERGE_AUTH_KEYS"] = "key-abc123"
	setEnv(t, e)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error with auth enabled and a key set, got: %v", err)
	}
	if !cfg.Auth.Enabled {
		t.Error("Auth.Enabled: want true, got false")
	}
	if len(cfg.Auth.Keys) != 1 {
		t.Fatalf("Auth.Keys: want 1 key, got %d", len(cfg.Auth.Keys))
	}
	if cfg.Auth.Keys[0] != "key-abc123" {
		t.Errorf("Auth.Keys[0]: want %q, got %q", "key-abc123", cfg.Auth.Keys[0])
	}
}

func TestLoad_Auth_Enabled_WithMultipleKeys_IsValid(t *testing.T) {
	clearVergeEnv(t)
	e := baseEnv()
	e["VERGE_AUTH_ENABLED"] = "true"
	e["VERGE_AUTH_KEYS"] = "key-abc123,key-def456"
	setEnv(t, e)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error with auth enabled and multiple keys, got: %v", err)
	}
	if len(cfg.Auth.Keys) != 2 {
		t.Fatalf("Auth.Keys: want 2 keys, got %d: %v", len(cfg.Auth.Keys), cfg.Auth.Keys)
	}
	if cfg.Auth.Keys[0] != "key-abc123" {
		t.Errorf("Auth.Keys[0]: want %q, got %q", "key-abc123", cfg.Auth.Keys[0])
	}
	if cfg.Auth.Keys[1] != "key-def456" {
		t.Errorf("Auth.Keys[1]: want %q, got %q", "key-def456", cfg.Auth.Keys[1])
	}
}

func TestLoad_Auth_Enabled_NoKeys_IsRejected(t *testing.T) {
	clearVergeEnv(t)
	e := baseEnv()
	e["VERGE_AUTH_ENABLED"] = "true"
	// VERGE_AUTH_KEYS deliberately omitted
	setEnv(t, e)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when auth is enabled but no keys are provided")
	}
}

func TestLoad_Auth_Enabled_EmptyKeys_IsRejected(t *testing.T) {
	clearVergeEnv(t)
	e := baseEnv()
	e["VERGE_AUTH_ENABLED"] = "true"
	e["VERGE_AUTH_KEYS"] = ""
	setEnv(t, e)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when auth is enabled but VERGE_AUTH_KEYS is empty")
	}
}

func TestLoad_Auth_Enabled_WhitespaceOnlyKeys_IsRejected(t *testing.T) {
	clearVergeEnv(t)
	e := baseEnv()
	e["VERGE_AUTH_ENABLED"] = "true"
	e["VERGE_AUTH_KEYS"] = "   "
	setEnv(t, e)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when auth is enabled but keys contain only whitespace")
	}
}

func TestLoad_Auth_Enabled_DirectValidate_IsRejected(t *testing.T) {
	cfg := &Config{
		Server: Server{
			HTTP: HTTP{Enabled: true, Port: 8080},
			GRPC: GRPC{Enabled: false, Port: 9090},
		},
		Storage: Storage{
			Postgres: PGConfig{URL: "postgres://u:p@localhost:5432/db?sslmode=disable"},
		},
		Outbox: OutboxConfig{
			SourceType:   "polling",
			PollInterval: 500 * time.Millisecond,
			BatchSize:    100,
			EventBus:     EventBusConfig{Type: "kafka"},
		},
		OTel: OTelConfig{
			Exporter:        "stdout",
			ServiceName:     "verge",
			SampleRate:      1.0,
			MetricsInterval: 15 * time.Second,
			LogLevel:        "info",
		},
		Auth: AuthConfig{
			Enabled: true,
			Keys:    nil, // no keys - must be rejected
		},
	}

	if err := validate(cfg); err == nil {
		t.Fatal("expected error when auth is enabled with nil keys")
	}
}

// full config round-trip

func TestLoad_FullValidConfig(t *testing.T) {
	clearVergeEnv(t)
	setEnv(t, map[string]string{
		"VERGE_SERVER_HTTP_ENABLED": "true",
		"VERGE_SERVER_HTTP_PORT":    "9000",
		"VERGE_SERVER_GRPC_ENABLED": "true",
		"VERGE_SERVER_GRPC_PORT":    "9001",

		"VERGE_STORAGE_POSTGRES_URL":     "postgres://u:p@db:5432/mydb?sslmode=disable",
		"VERGE_STORAGE_REDIS_ENABLED":    "true",
		"VERGE_STORAGE_REDIS_URL":        "redis://localhost:6379",
		"VERGE_STORAGE_REDIS_BRANCH_TTL": "1m",
		"VERGE_STORAGE_NEO4J_ENABLED":    "true",
		"VERGE_STORAGE_NEO4J_URL":        "bolt://localhost:7687",

		"VERGE_OUTBOX_SOURCE_TYPE":      "polling",
		"VERGE_OUTBOX_POLL_INTERVAL":    "250ms",
		"VERGE_OUTBOX_BATCH_SIZE":       "200",
		"VERGE_OUTBOX_EVENTBUS_ENABLED": "true",
		"VERGE_OUTBOX_EVENTBUS_TYPE":    "kafka",

		"VERGE_KAFKA_BROKERS": "kafka:9092",
		"VERGE_KAFKA_TOPIC":   "verge.prod.events",

		"VERGE_OTEL_ENABLED":          "true",
		"VERGE_OTEL_EXPORTER":         "prometheus",
		"VERGE_OTEL_SERVICE_NAME":     "verge-prod",
		"VERGE_OTEL_SAMPLE_RATE":      "0.5",
		"VERGE_OTEL_METRICS_INTERVAL": "30s",
		"VERGE_OTEL_LOG_LEVEL":        "debug",

		"VERGE_AUTH_ENABLED": "true",
		"VERGE_AUTH_KEYS":    "key-prod-primary,key-prod-secondary",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// server
	if cfg.Server.HTTP.Port != 9000 {
		t.Errorf("Server.HTTP.Port: want 9000, got %d", cfg.Server.HTTP.Port)
	}
	if cfg.Server.GRPC.Port != 9001 {
		t.Errorf("Server.GRPC.Port: want 9001, got %d", cfg.Server.GRPC.Port)
	}

	// storage
	if cfg.Storage.Postgres.URL != "postgres://u:p@db:5432/mydb?sslmode=disable" {
		t.Errorf("Storage.Postgres.URL: unexpected value %q", cfg.Storage.Postgres.URL)
	}
	if !cfg.Storage.Redis.Enabled {
		t.Error("Storage.Redis.Enabled: want true, got false")
	}
	if cfg.Storage.Redis.URL != "redis://localhost:6379" {
		t.Errorf("Storage.Redis.URL: unexpected value %q", cfg.Storage.Redis.URL)
	}
	if cfg.Storage.Redis.BranchTTL != time.Minute {
		t.Errorf("Storage.Redis.BranchTTL: want 1m, got %s", cfg.Storage.Redis.BranchTTL)
	}
	if !cfg.Storage.Neo4j.Enabled {
		t.Error("Storage.Neo4j.Enabled: want true, got false")
	}
	if cfg.Storage.Neo4j.URL != "bolt://localhost:7687" {
		t.Errorf("Storage.Neo4j.URL: unexpected value %q", cfg.Storage.Neo4j.URL)
	}

	// outbox
	if cfg.Outbox.SourceType != "polling" {
		t.Errorf("Outbox.SourceType: want %q, got %q", "polling", cfg.Outbox.SourceType)
	}
	if cfg.Outbox.PollInterval != 250*time.Millisecond {
		t.Errorf("Outbox.PollInterval: want 250ms, got %s", cfg.Outbox.PollInterval)
	}
	if cfg.Outbox.BatchSize != 200 {
		t.Errorf("Outbox.BatchSize: want 200, got %d", cfg.Outbox.BatchSize)
	}
	if !cfg.Outbox.EventBus.Enabled {
		t.Error("Outbox.EventBus.Enabled: want true, got false")
	}
	if cfg.Outbox.EventBus.Type != "kafka" {
		t.Errorf("Outbox.EventBus.Type: want %q, got %q", "kafka", cfg.Outbox.EventBus.Type)
	}

	// kafka
	if cfg.Kafka.Brokers != "kafka:9092" {
		t.Errorf("Kafka.Brokers: want %q, got %q", "kafka:9092", cfg.Kafka.Brokers)
	}
	if cfg.Kafka.Topic != "verge.prod.events" {
		t.Errorf("Kafka.Topic: want %q, got %q", "verge.prod.events", cfg.Kafka.Topic)
	}

	// otel
	if !cfg.OTel.Enabled {
		t.Error("OTel.Enabled: want true, got false")
	}
	if cfg.OTel.Exporter != "prometheus" {
		t.Errorf("OTel.Exporter: want %q, got %q", "prometheus", cfg.OTel.Exporter)
	}
	if cfg.OTel.ServiceName != "verge-prod" {
		t.Errorf("OTel.ServiceName: want %q, got %q", "verge-prod", cfg.OTel.ServiceName)
	}
	if cfg.OTel.SampleRate != 0.5 {
		t.Errorf("OTel.SampleRate: want 0.5, got %f", cfg.OTel.SampleRate)
	}
	if cfg.OTel.MetricsInterval != 30*time.Second {
		t.Errorf("OTel.MetricsInterval: want 30s, got %s", cfg.OTel.MetricsInterval)
	}
	if cfg.OTel.LogLevel != "debug" {
		t.Errorf("OTel.LogLevel: want %q, got %q", "debug", cfg.OTel.LogLevel)
	}

	// auth
	if !cfg.Auth.Enabled {
		t.Error("Auth.Enabled: want true, got false")
	}
	if len(cfg.Auth.Keys) != 2 {
		t.Fatalf("Auth.Keys: want 2 keys, got %d: %v", len(cfg.Auth.Keys), cfg.Auth.Keys)
	}
	if cfg.Auth.Keys[0] != "key-prod-primary" {
		t.Errorf("Auth.Keys[0]: want %q, got %q", "key-prod-primary", cfg.Auth.Keys[0])
	}
	if cfg.Auth.Keys[1] != "key-prod-secondary" {
		t.Errorf("Auth.Keys[1]: want %q, got %q", "key-prod-secondary", cfg.Auth.Keys[1])
	}
}

func TestLoad_FullValidConfig_Debezium(t *testing.T) {
	clearVergeEnv(t)
	setEnv(t, map[string]string{
		"VERGE_SERVER_HTTP_ENABLED": "true",
		"VERGE_SERVER_HTTP_PORT":    "8080",

		"VERGE_STORAGE_POSTGRES_URL": "postgres://u:p@db:5432/mydb?sslmode=disable",

		"VERGE_OUTBOX_SOURCE_TYPE":       "debezium",
		"VERGE_OUTBOX_BATCH_SIZE":        "1000",
		"VERGE_OUTBOX_DEBEZIUM_BROKERS":  "kafka-1:9092,kafka-2:9092",
		"VERGE_OUTBOX_DEBEZIUM_TOPIC":    "verge.outbox.events",
		"VERGE_OUTBOX_DEBEZIUM_GROUP_ID": "verge-worker-prod",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Outbox.SourceType != "debezium" {
		t.Errorf("Outbox.SourceType: want %q, got %q", "debezium", cfg.Outbox.SourceType)
	}
	if cfg.Outbox.BatchSize != 1000 {
		t.Errorf("Outbox.BatchSize: want 1000, got %d", cfg.Outbox.BatchSize)
	}
	if cfg.Outbox.DebeziumBrokers != "kafka-1:9092,kafka-2:9092" {
		t.Errorf("Outbox.DebeziumBrokers: want %q, got %q",
			"kafka-1:9092,kafka-2:9092", cfg.Outbox.DebeziumBrokers)
	}
	if cfg.Outbox.DebeziumTopic != "verge.outbox.events" {
		t.Errorf("Outbox.DebeziumTopic: want %q, got %q",
			"verge.outbox.events", cfg.Outbox.DebeziumTopic)
	}
	if cfg.Outbox.DebeziumGroupID != "verge-worker-prod" {
		t.Errorf("Outbox.DebeziumGroupID: want %q, got %q",
			"verge-worker-prod", cfg.Outbox.DebeziumGroupID)
	}
}

func TestLoad_AllStorageEnabled(t *testing.T) {
	clearVergeEnv(t)
	setEnv(t, map[string]string{
		"VERGE_SERVER_HTTP_ENABLED":   "true",
		"VERGE_STORAGE_POSTGRES_URL":  "postgres://u:p@localhost:5432/db?sslmode=disable",
		"VERGE_STORAGE_REDIS_ENABLED": "true",
		"VERGE_STORAGE_REDIS_URL":     "redis://localhost:6379",
		"VERGE_STORAGE_NEO4J_ENABLED": "true",
		"VERGE_STORAGE_NEO4J_URL":     "bolt://localhost:7687",
	})

	_, err := Load()
	if err != nil {
		t.Fatalf("expected no error with all storage backends enabled, got: %v", err)
	}
}
