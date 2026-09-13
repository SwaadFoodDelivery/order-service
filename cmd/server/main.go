package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"order-service/infra/postgres"
	grpcserver "order-service/internal/grpc/server"
	"order-service/internal/services/order/repository"
	"order-service/pkg/config"
	"order-service/pkg/logger"
)

func main() {
	if err := run(); err != nil {
		// run returns sanitized configuration/startup errors only.
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log, err := logger.New(cfg.App.LogLevel)
	if err != nil {
		return fmt.Errorf("initialize logger failed")
	}
	defer log.Sync()
	startup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	db, err := postgres.NewPostgresDB(startup, cfg)
	cancel()
	if err != nil {
		return err
	}
	defer db.Close()
	s, err := grpcserver.New(cfg, repository.NewPostgresRepository(db), log)
	if err != nil {
		return err
	}
	lis, err := net.Listen("tcp", cfg.GRPC.OrderAddr)
	if err != nil {
		return fmt.Errorf("listen on configured order RPC address failed")
	}
	defer lis.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	serveErr := make(chan error, 1)
	go func() { serveErr <- s.Serve(lis) }()
	log.Info("read-only GetOrder RPC listening")
	select {
	case <-ctx.Done():
	case <-serveErr:
		s.Stop()
		return fmt.Errorf("order RPC server stopped unexpectedly")
	}
	done := make(chan struct{})
	go func() { s.GracefulStop(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		s.Stop()
	}
	return nil
}
