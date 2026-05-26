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

	"VERGE_STORAGE_POSTGRES_URL": {},

	"VERGE_STORAGE_REDIS_ENABLED":    {},
	"VERGE_STORAGE_REDIS_URL":        {},
	"VERGE_STORAGE_REDIS_BRANCH_TTL": {},

	"VERGE_STORAGE_NEO4J_ENABLED": {},
	"VERGE_STORAGE_NEO4J_URL":     {},

	// Outbox worker
	"VERGE_OUTBOX_SOURCE_TYPE":       {}, // NEW: "polling" or "debezium"
	"VERGE_OUTBOX_POLL_INTERVAL":     {},
	"VERGE_OUTBOX_BATCH_SIZE":        {},
	"VERGE_OUTBOX_EVENTBUS_ENABLED":  {},
	"VERGE_OUTBOX_EVENTBUS_TYPE":     {},
	"VERGE_OUTBOX_DEBEZIUM_BROKERS":  {}, // NEW
	"VERGE_OUTBOX_DEBEZIUM_TOPIC":    {}, // NEW
	"VERGE_OUTBOX_DEBEZIUM_GROUP_ID": {}, // NEW

	"VERGE_KAFKA_BROKERS": {},
	"VERGE_KAFKA_TOPIC":   {},
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
	// Construct the config directly to test validation in isolation.
	// SourceType must be valid to isolate the EventBus.Type check.
	cfg := &Config{
		Server: Server{
			HTTP: HTTP{Enabled: true, Port: 8080},
			GRPC: GRPC{Enabled: false, Port: 9090},
		},
		Storage: Storage{
			Postgres: PGConfig{URL: "postgres://u:p@localhost:5432/db?sslmode=disable"},
		},
		Outbox: OutboxConfig{
			SourceType:   "polling", // must be valid to reach EventBus check
			PollInterval: 500 * time.Millisecond,
			BatchSize:    100,
			EventBus: EventBusConfig{
				Enabled: true,
				Type:    "", // explicitly empty - this is what we're testing
			},
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
