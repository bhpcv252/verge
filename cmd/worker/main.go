package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/bhpcv252/verge/internal/config"
	"github.com/bhpcv252/verge/internal/observability"
	"github.com/bhpcv252/verge/internal/outbox"
	"github.com/bhpcv252/verge/internal/outbox/eventbus/kafka"
	neo4jstore "github.com/bhpcv252/verge/internal/storage/neo4j"
	"github.com/bhpcv252/verge/internal/storage/postgres"
	redisstore "github.com/bhpcv252/verge/internal/storage/redis"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("fatal: %v", err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	obs, err := observability.New(cfg.OTel)
	if err != nil {
		return fmt.Errorf("observability: %w", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := obs.Shutdown(ctx); err != nil {
			log.Printf("observability shutdown error: %v", err)
		}
	}()

	logger := obs.Logger.With(slog.String("component", "worker"))
	logger.Info("starting outbox worker",
		slog.Bool("otel_enabled", cfg.OTel.Enabled),
		slog.String("source_type", cfg.Outbox.SourceType),
		slog.Bool("eventbus_enabled", cfg.Outbox.EventBus.Enabled),
		slog.Int("batch_size", cfg.Outbox.BatchSize),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// signal handling
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		sig := <-quit
		logger.Info("received signal", slog.String("signal", sig.String()))
		cancel()
	}()

	var pool *pgxpool.Pool
	if cfg.Outbox.SourceType == "polling" || cfg.Storage.Neo4j.Enabled ||
		cfg.Storage.Redis.Enabled {
		pool, err = postgres.NewPool(ctx, cfg.Storage.Postgres.URL)
		if err != nil {
			return fmt.Errorf("postgres: %w", err)
		}
		defer pool.Close()
	}

	var handlers []outbox.OutboxHandler

	if cfg.Storage.Neo4j.Enabled {
		driver, err := neo4jstore.NewDriver(ctx, cfg.Storage.Neo4j.URL)
		if err != nil {
			return fmt.Errorf("neo4j: %w", err)
		}
		defer driver.Close(ctx)
		handlers = append(handlers, outbox.NewNeo4jHandler(driver))
		logger.Info("neo4j handler registered",
			slog.String("event_type", "CommitCreated"),
		)
	}

	if cfg.Storage.Redis.Enabled {
		rdb, err := redisstore.NewClient(ctx, cfg.Storage.Redis.URL)
		if err != nil {
			return fmt.Errorf("redis: %w", err)
		}
		defer rdb.Close()
		healStore := redisstore.NewBranchHeadStore(rdb, cfg.Storage.Redis.BranchTTL)
		handlers = append(handlers, outbox.NewRedisHealHandler(healStore))
		logger.Info("redis heal handler registered",
			slog.String("event_type", "BranchHeadMoved"),
		)
	}

	var source outbox.EventSource
	switch cfg.Outbox.SourceType {
	case "polling":
		if pool == nil {
			return fmt.Errorf("polling source requires PostgreSQL connection")
		}
		source = outbox.NewPollingSource(pool, cfg.Outbox.PollInterval, cfg.Outbox.BatchSize)
		logger.Info("using polling source",
			slog.String("interval", cfg.Outbox.PollInterval.String()),
			slog.Int("batch", cfg.Outbox.BatchSize),
		)
	case "debezium":
		if cfg.Outbox.DebeziumBrokers == "" {
			return fmt.Errorf("VERGE_OUTBOX_DEBEZIUM_BROKERS is required when SOURCE_TYPE=debezium")
		}
		brokers := strings.Split(cfg.Outbox.DebeziumBrokers, ",")
		source = outbox.NewDebeziumSource(outbox.DebeziumConfig{
			Brokers: brokers,
			Topic:   cfg.Outbox.DebeziumTopic,
			GroupID: cfg.Outbox.DebeziumGroupID,
			Batch:   cfg.Outbox.BatchSize,
		})
		logger.Info("using debezium source",
			slog.String("topic", cfg.Outbox.DebeziumTopic),
			slog.String("group", cfg.Outbox.DebeziumGroupID),
			slog.Any("brokers", brokers),
		)
	default:
		return fmt.Errorf(
			"unknown source type: %s (valid: polling, debezium)",
			cfg.Outbox.SourceType,
		)
	}

	// build worker options
	opts := []outbox.Option{
		outbox.WithObservability(obs),
		outbox.WithSource(source),
		outbox.WithHandlers(handlers),
	}

	if cfg.Outbox.EventBus.Enabled {
		if cfg.Outbox.EventBus.Type != "kafka" {
			return fmt.Errorf("unsupported event bus type: %q", cfg.Outbox.EventBus.Type)
		}
		if cfg.Kafka.Brokers == "" {
			return fmt.Errorf("VERGE_KAFKA_BROKERS is required when EventBus is enabled")
		}
		brokers := strings.Split(cfg.Kafka.Brokers, ",")
		producer := kafka.NewProducer(kafka.Config{
			Brokers: brokers,
			Topic:   cfg.Kafka.Topic,
		})
		defer producer.Close()
		opts = append(opts, outbox.WithEventBus(producer))
		logger.Info("kafka event bus enabled",
			slog.String("topic", cfg.Kafka.Topic),
			slog.Any("brokers", brokers),
		)
	}

	// run worker
	worker := outbox.NewWorker(opts...)
	if err := worker.Run(ctx); err != nil {
		return fmt.Errorf("worker error: %w", err)
	}

	return nil
}
