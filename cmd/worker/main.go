package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/bhpcv252/verge/internal/config"
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// signal handling
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		sig := <-quit
		log.Printf("received signal: %s", sig)
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
		log.Println("outbox worker: Neo4j handler registered (CommitCreated)")
	}

	if cfg.Storage.Redis.Enabled {
		rdb, err := redisstore.NewClient(ctx, cfg.Storage.Redis.URL)
		if err != nil {
			return fmt.Errorf("redis: %w", err)
		}
		defer rdb.Close()

		healStore := redisstore.NewBranchHeadStore(rdb, cfg.Storage.Redis.BranchTTL)
		handlers = append(handlers, outbox.NewRedisHealHandler(healStore))
		log.Println("outbox worker: Redis heal handler registered (BranchHeadMoved)")
	}

	var source outbox.EventSource

	switch cfg.Outbox.SourceType {
	case "polling":
		if pool == nil {
			return fmt.Errorf("polling source requires PostgreSQL connection")
		}
		source = outbox.NewPollingSource(pool, cfg.Outbox.PollInterval, cfg.Outbox.BatchSize)
		log.Printf("outbox worker: using polling source (interval=%s, batch=%d)",
			cfg.Outbox.PollInterval, cfg.Outbox.BatchSize)

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
		log.Printf("outbox worker: using debezium source (topic=%s, group=%s, brokers=%v)",
			cfg.Outbox.DebeziumTopic, cfg.Outbox.DebeziumGroupID, brokers)

	default:
		return fmt.Errorf(
			"unknown source type: %s (valid: polling, debezium)",
			cfg.Outbox.SourceType,
		)
	}

	// build worker options
	opts := []outbox.Option{
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
		log.Printf("outbox worker: EventBus mode - publishing to Kafka topic %q", cfg.Kafka.Topic)
	}

	// run worker
	log.Printf("outbox worker: batch_size=%d", cfg.Outbox.BatchSize)

	worker := outbox.NewWorker(opts...)
	if err := worker.Run(ctx); err != nil {
		return fmt.Errorf("worker error: %w", err)
	}

	return nil
}
