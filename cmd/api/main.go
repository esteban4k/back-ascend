package main

import (
	"ascend/internal/infrastructure/config"
	"ascend/internal/infrastructure/container"
	"ascend/internal/infrastructure/httpapi"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata"
)

func main() {
	if err := run(); err != nil {
		slog.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	c, err := container.Build(ctx, cfg)
	if err != nil {
		return err
	}
	defer c.Close()

	server := &http.Server{
		Addr: cfg.Addr,
		Handler: httpapi.NewHandler(c.UseCases, httpapi.Options{
			AllowedOrigins: cfg.AllowedOrigins,
			APIToken:       cfg.APIToken,
			Reset:          c.Reset,
			DataSource:     c.DataSource,
			Ping:           c.Ping,
			Logger:         logger,
		}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errs := make(chan error, 1)
	go func() {
		logger.Info("listening", "addr", cfg.Addr, "store", c.DataSource, "timezone", cfg.Location.String(), "seed", cfg.Seed)
		errs <- server.ListenAndServe()
	}()

	select {
	case err := <-errs:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
	}

	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return server.Shutdown(shutdownCtx)
}
