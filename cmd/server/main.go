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
	"github.com/bhpcv252/verge/internal/storage/postgres"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("fatal: %v", err)
	}
}

func run() error {
	// Config
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	// Database
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool, err := postgres.NewPool(ctx, cfg.Storage.Postgres.URL)
	if err != nil {
		return fmt.Errorf("postgres: %w", err)
	}
	defer pool.Close()

	// Stores
	repoStore := postgres.NewRepoStore(pool)
	branchStore := postgres.NewBranchStore(pool)
	commitStore := postgres.NewCommitStore(pool)

	// Services
	repoSvc := service.NewRepoService(repoStore)
	commitSvc := service.NewCommitService(commitStore, repoStore)
	branchSvc := service.NewBranchService(branchStore, repoStore, commitStore)
	mergeSvc := service.NewMergeService(commitStore, repoStore, commitStore, branchStore)

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

	// shutdown signal
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
