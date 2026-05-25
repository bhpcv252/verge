package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"

	grpcv1 "github.com/bhpcv252/verge/internal/api/grpc/v1"
	restv1 "github.com/bhpcv252/verge/internal/api/rest/v1"
	"github.com/bhpcv252/verge/internal/config"
	"github.com/bhpcv252/verge/internal/service"
	"github.com/bhpcv252/verge/internal/storage/composite"
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

	// PostgreSQL (always required)
	pool, err := postgres.NewPool(ctx, cfg.Storage.Postgres.URL)
	if err != nil {
		return fmt.Errorf("postgres: %w", err)
	}
	defer pool.Close()

	// Base postgres stores
	pgBranchStore := postgres.NewBranchStore(pool)
	pgCommitStore := postgres.NewCommitStore(pool)
	repoStore := postgres.NewRepoStore(pool)

	// Redis (optional)
	// when enabled, branch heads and commit objects are cached in Redis
	var (
		branchStore service.BranchStore = pgBranchStore
		commitStore service.CommitStore = pgCommitStore
	)

	if cfg.Storage.Redis.Enabled {
		rdb, err := redisstore.NewClient(ctx, cfg.Storage.Redis.URL)
		if err != nil {
			return fmt.Errorf("redis: %w", err)
		}
		defer rdb.Close()

		redisBranchHead := redisstore.NewBranchHeadStore(rdb, cfg.Storage.Redis.BranchTTL)
		redisCommitCache := redisstore.NewCommitCache(rdb)

		branchStore = composite.NewBranchRouter(pgBranchStore, redisBranchHead)
		commitStore = composite.NewCommitRouter(pgCommitStore, redisCommitCache)

		log.Printf("redis: branch TTL=%s, commit cache enabled", cfg.Storage.Redis.BranchTTL)
	}

	// Neo4j (optional)
	// when enabled, DAG traversal queries use Neo4j Cypher with postgres fallback
	if cfg.Storage.Neo4j.Enabled {
		driver, err := neo4jstore.NewDriver(ctx, cfg.Storage.Neo4j.URL)
		if err != nil {
			return fmt.Errorf("neo4j: %w", err)
		}
		defer driver.Close(ctx)

		pgGraphStore := postgres.NewGraphStore(pool)
		neo4jGraphStore := neo4jstore.NewGraphStore(driver)
		_ = composite.NewGraphRouter(
			neo4jGraphStore,
			pgGraphStore,
		)

		log.Println("neo4j: graph projection enabled")
	}

	// Services
	repoSvc := service.NewRepoService(repoStore)
	commitSvc := service.NewCommitService(commitStore, repoStore)
	branchSvc := service.NewBranchService(branchStore, repoStore, commitStore)
	mergeSvc := service.NewMergeService(pgCommitStore, repoStore, commitStore, branchStore)

	g, gCtx := errgroup.WithContext(ctx)

	// HTTP
	if cfg.Server.HTTP.Enabled {
		router := restv1.NewRouter(
			restv1.NewRepoHandler(repoSvc),
			restv1.NewBranchHandler(branchSvc),
			restv1.NewCommitHandler(commitSvc),
			restv1.NewMergeHandler(mergeSvc),
		)
		httpSrv := &http.Server{
			Addr:         fmt.Sprintf(":%d", cfg.Server.HTTP.Port),
			Handler:      router,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
			IdleTimeout:  60 * time.Second,
		}

		g.Go(func() error {
			log.Printf("HTTP server listening on :%d", cfg.Server.HTTP.Port)
			if err := httpSrv.ListenAndServe(); err != nil &&
				!errors.Is(err, http.ErrServerClosed) {
				return fmt.Errorf("HTTP server: %w", err)
			}
			return nil
		})
		g.Go(func() error {
			<-gCtx.Done()
			log.Println("HTTP server: shutting down")
			shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer shutCancel()
			return httpSrv.Shutdown(shutCtx)
		})
	}

	// gRPC
	if cfg.Server.GRPC.Enabled {
		grpcSrv := grpcv1.NewServer(
			grpcv1.NewRepoServer(repoSvc),
			grpcv1.NewBranchServer(branchSvc),
			grpcv1.NewCommitServer(commitSvc),
			grpcv1.NewMergeServer(mergeSvc),
		)

		g.Go(func() error {
			lis, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.Server.GRPC.Port))
			if err != nil {
				return fmt.Errorf("gRPC listener: %w", err)
			}
			log.Printf("gRPC server listening on :%d", cfg.Server.GRPC.Port)
			if err := grpcSrv.Serve(lis); err != nil {
				return fmt.Errorf("gRPC server: %w", err)
			}
			return nil
		})
		g.Go(func() error {
			<-gCtx.Done()
			log.Println("gRPC server: shutting down")
			grpcSrv.GracefulStop()
			return nil
		})
	}

	// Shutdown signal
	g.Go(func() error {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		select {
		case sig := <-quit:
			log.Printf("received signal: %s", sig)
			cancel()
		case <-gCtx.Done():
		}
		return nil
	})

	return g.Wait()
}
