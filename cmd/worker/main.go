package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

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

	// postgreSQL
	pool, err := postgres.NewPool(ctx, cfg.Storage.Postgres.URL)
	if err != nil {
		return fmt.Errorf("postgres: %w", err)
	}
	defer pool.Close()

	// build handlers
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

	// build worker options
	opts := []outbox.Option{
		outbox.WithHandlers(handlers),
		outbox.WithInterval(cfg.Outbox.PollInterval),
		outbox.WithBatchSize(cfg.Outbox.BatchSize),
	}

	// EventBus
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

	// run
	log.Printf("outbox worker: poll_interval=%s batch_size=%d",
		cfg.Outbox.PollInterval, cfg.Outbox.BatchSize)

	worker := outbox.NewWorker(pool, opts...)
	worker.Run(ctx) // blocks until ctx cancelled

	return nil
}
