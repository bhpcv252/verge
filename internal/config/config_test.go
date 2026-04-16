package config

import (
	"os"
	"strings"
	"testing"
)

// sets multiple environment variables and returns a cleanup function
func setEnv(t *testing.T, pairs map[string]string) {
	t.Helper()
	for k, v := range pairs {
		t.Setenv(k, v)
	}
}

// Exhaustive list of all VERGE_ variables recognized by this service.
// If you add a new env var to config.go, add it here as well with a test case.
var knownVergeKeys = map[string]struct{}{
	"VERGE_SERVER_HTTP_ENABLED":   {},
	"VERGE_SERVER_HTTP_PORT":      {},
	"VERGE_SERVER_GRPC_ENABLED":   {},
	"VERGE_SERVER_GRPC_PORT":      {},
	"VERGE_STORAGE_POSTGRES_URL":  {},
	"VERGE_STORAGE_REDIS_ENABLED": {},
	"VERGE_STORAGE_REDIS_URL":     {},
	"VERGE_STORAGE_NEO4J_ENABLED": {},
	"VERGE_STORAGE_NEO4J_URL":     {},
}

// returns the minimal valid environment needed to pass all validations
func baseEnv() map[string]string {
	return map[string]string{
		"VERGE_SERVER_HTTP_ENABLED":   "true",
		"VERGE_SERVER_HTTP_PORT":      "8080",
		"VERGE_SERVER_GRPC_ENABLED":   "false",
		"VERGE_SERVER_GRPC_PORT":      "9090",
		"VERGE_STORAGE_POSTGRES_URL":  "postgres://verge:changeme@postgres:5432/verge?sslmode=disable",
		"VERGE_STORAGE_REDIS_ENABLED": "false",
		"VERGE_STORAGE_REDIS_URL":     "",
		"VERGE_STORAGE_NEO4J_ENABLED": "false",
		"VERGE_STORAGE_NEO4J_URL":     "",
	}
}

// removes all VERGE_ prefixed env vars so each test starts clean
func clearVergeEnv(t *testing.T) {
	t.Helper()
	keys := []string{
		"VERGE_SERVER_HTTP_ENABLED",
		"VERGE_SERVER_HTTP_PORT",
		"VERGE_SERVER_GRPC_ENABLED",
		"VERGE_SERVER_GRPC_PORT",
		"VERGE_STORAGE_POSTGRES_URL",
		"VERGE_STORAGE_REDIS_ENABLED",
		"VERGE_STORAGE_REDIS_URL",
		"VERGE_STORAGE_NEO4J_ENABLED",
		"VERGE_STORAGE_NEO4J_URL",
	}
	for _, k := range keys {
		os.Unsetenv(k)
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
				"unrecognised VERGE_ env var %q — either wire it up in config.go or remove it",
				key,
			)
		}
	}
}

func TestLoad_Defaults(t *testing.T) {
	clearVergeEnv(t)
	setEnv(t, map[string]string{
		"VERGE_STORAGE_POSTGRES_URL": "postgres://verge:changeme@localhost:5432/verge?sslmode=disable",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if !cfg.Server.HTTP.Enabled {
		t.Error("expected HTTP.Enabled default to be true")
	}
	if cfg.Server.HTTP.Port != 8080 {
		t.Errorf("expected HTTP.Port default 8080, got %d", cfg.Server.HTTP.Port)
	}
	if cfg.Server.GRPC.Enabled {
		t.Error("expected GRPC.Enabled default to be false")
	}
	if cfg.Server.GRPC.Port != 9090 {
		t.Errorf("expected GRPC.Port default 9090, got %d", cfg.Server.GRPC.Port)
	}
	if cfg.Storage.Redis.Enabled {
		t.Error("expected Redis.Enabled default to be false")
	}
	if cfg.Storage.Neo4j.Enabled {
		t.Error("expected Neo4j.Enabled default to be false")
	}
}

func TestLoad_FullValidConfig(t *testing.T) {
	clearVergeEnv(t)
	setEnv(t, map[string]string{
		"VERGE_SERVER_HTTP_ENABLED":   "true",
		"VERGE_SERVER_HTTP_PORT":      "9000",
		"VERGE_SERVER_GRPC_ENABLED":   "true",
		"VERGE_SERVER_GRPC_PORT":      "9001",
		"VERGE_STORAGE_POSTGRES_URL":  "postgres://u:p@db:5432/mydb?sslmode=disable",
		"VERGE_STORAGE_REDIS_ENABLED": "true",
		"VERGE_STORAGE_REDIS_URL":     "redis://localhost:6379",
		"VERGE_STORAGE_NEO4J_ENABLED": "true",
		"VERGE_STORAGE_NEO4J_URL":     "bolt://localhost:7687",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Server.HTTP.Port != 9000 {
		t.Errorf("expected HTTP port 9000, got %d", cfg.Server.HTTP.Port)
	}
	if cfg.Server.GRPC.Port != 9001 {
		t.Errorf("expected GRPC port 9001, got %d", cfg.Server.GRPC.Port)
	}
	if cfg.Storage.Postgres.URL != "postgres://u:p@db:5432/mydb?sslmode=disable" {
		t.Errorf("unexpected Postgres URL: %s", cfg.Storage.Postgres.URL)
	}
	if !cfg.Storage.Redis.Enabled {
		t.Error("expected Redis to be enabled")
	}
	if cfg.Storage.Redis.URL != "redis://localhost:6379" {
		t.Errorf("unexpected Redis URL: %s", cfg.Storage.Redis.URL)
	}
	if !cfg.Storage.Neo4j.Enabled {
		t.Error("expected Neo4j to be enabled")
	}
	if cfg.Storage.Neo4j.URL != "bolt://localhost:7687" {
		t.Errorf("unexpected Neo4j URL: %s", cfg.Storage.Neo4j.URL)
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

func TestLoad_InvalidHTTPPort_Zero(t *testing.T) {
	clearVergeEnv(t)
	env := baseEnv()
	env["VERGE_SERVER_HTTP_PORT"] = "0"
	setEnv(t, env)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for HTTP port 0")
	}
}

func TestLoad_InvalidHTTPPort_Max(t *testing.T) {
	clearVergeEnv(t)
	env := baseEnv()
	env["VERGE_SERVER_HTTP_PORT"] = "65535"
	setEnv(t, env)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for HTTP port 65535 (must be lt=65535)")
	}
}

func TestLoad_InvalidGRPCPort_Zero(t *testing.T) {
	clearVergeEnv(t)
	env := baseEnv()
	env["VERGE_SERVER_GRPC_ENABLED"] = "true"
	env["VERGE_SERVER_GRPC_PORT"] = "0"
	setEnv(t, env)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for GRPC port 0")
	}
}

func TestLoad_ValidPortBoundaries(t *testing.T) {
	clearVergeEnv(t)
	env := baseEnv()
	env["VERGE_SERVER_HTTP_PORT"] = "1"     // minimum (gt=0)
	env["VERGE_SERVER_GRPC_PORT"] = "65534" // maximum (lt=65535)
	env["VERGE_SERVER_GRPC_ENABLED"] = "true"
	setEnv(t, env)

	_, err := Load()
	if err != nil {
		t.Fatalf("expected no error at port boundaries, got: %v", err)
	}
}

func TestLoad_MissingPostgresURL(t *testing.T) {
	clearVergeEnv(t)
	setEnv(t, map[string]string{
		"VERGE_SERVER_HTTP_ENABLED": "true",
	})

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when Postgres URL is missing")
	}
}

func TestLoad_InvalidPostgresURL(t *testing.T) {
	clearVergeEnv(t)
	env := baseEnv()
	env["VERGE_STORAGE_POSTGRES_URL"] = "not-a-valid-url"
	setEnv(t, env)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid Postgres URL")
	}
}

func TestLoad_RedisEnabledWithURL(t *testing.T) {
	clearVergeEnv(t)
	env := baseEnv()
	env["VERGE_STORAGE_REDIS_ENABLED"] = "true"
	env["VERGE_STORAGE_REDIS_URL"] = "redis://localhost:6379"
	setEnv(t, env)

	_, err := Load()
	if err != nil {
		t.Fatalf("expected no error with Redis enabled and URL set, got: %v", err)
	}
}

func TestLoad_RedisEnabledWithoutURL(t *testing.T) {
	clearVergeEnv(t)
	env := baseEnv()
	env["VERGE_STORAGE_REDIS_ENABLED"] = "true"
	env["VERGE_STORAGE_REDIS_URL"] = ""
	setEnv(t, env)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when Redis is enabled but URL is empty")
	}
}

func TestLoad_RedisDisabledWithoutURL(t *testing.T) {
	clearVergeEnv(t)
	env := baseEnv()
	env["VERGE_STORAGE_REDIS_ENABLED"] = "false"
	env["VERGE_STORAGE_REDIS_URL"] = ""
	setEnv(t, env)

	_, err := Load()
	if err != nil {
		t.Fatalf("expected no error when Redis is disabled and URL is empty, got: %v", err)
	}
}

func TestLoad_RedisDisabledWithURL(t *testing.T) {
	// URL present but Redis disabled — should be fine (omitempty)
	clearVergeEnv(t)
	env := baseEnv()
	env["VERGE_STORAGE_REDIS_ENABLED"] = "false"
	env["VERGE_STORAGE_REDIS_URL"] = "redis://localhost:6379"
	setEnv(t, env)

	_, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoad_RedisEnabledInvalidURL(t *testing.T) {
	clearVergeEnv(t)
	env := baseEnv()
	env["VERGE_STORAGE_REDIS_ENABLED"] = "true"
	env["VERGE_STORAGE_REDIS_URL"] = "not-a-url"
	setEnv(t, env)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid Redis URL")
	}
}

func TestLoad_Neo4jEnabledWithURL(t *testing.T) {
	clearVergeEnv(t)
	env := baseEnv()
	env["VERGE_STORAGE_NEO4J_ENABLED"] = "true"
	env["VERGE_STORAGE_NEO4J_URL"] = "bolt://localhost:7687"
	setEnv(t, env)

	_, err := Load()
	if err != nil {
		t.Fatalf("expected no error with Neo4j enabled and URL set, got: %v", err)
	}
}

func TestLoad_Neo4jEnabledWithoutURL(t *testing.T) {
	clearVergeEnv(t)
	env := baseEnv()
	env["VERGE_STORAGE_NEO4J_ENABLED"] = "true"
	env["VERGE_STORAGE_NEO4J_URL"] = ""
	setEnv(t, env)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when Neo4j is enabled but URL is empty")
	}
}

func TestLoad_Neo4jDisabledWithoutURL(t *testing.T) {
	clearVergeEnv(t)
	env := baseEnv()
	env["VERGE_STORAGE_NEO4J_ENABLED"] = "false"
	env["VERGE_STORAGE_NEO4J_URL"] = ""
	setEnv(t, env)

	_, err := Load()
	if err != nil {
		t.Fatalf("expected no error when Neo4j is disabled and URL is empty, got: %v", err)
	}
}

func TestLoad_Neo4jEnabledInvalidURL(t *testing.T) {
	clearVergeEnv(t)
	env := baseEnv()
	env["VERGE_STORAGE_NEO4J_ENABLED"] = "true"
	env["VERGE_STORAGE_NEO4J_URL"] = "not-a-url"
	setEnv(t, env)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid Neo4j URL")
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
		t.Fatalf("expected no error with all storage enabled, got: %v", err)
	}
}
