package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hughdo/cocode/services/cocoded/internal/app"
	"github.com/hughdo/cocode/services/cocoded/internal/httpapi"
)

func main() {
	config, err := app.LoadConfig()
	if err != nil {
		panic(err)
	}

	logger, cleanup, err := app.NewLogger(config.LogPath)
	if err != nil {
		panic(err)
	}
	defer cleanup()

	router := httpapi.NewRouter(config, logger)
	server := &http.Server{
		Addr:              config.Addr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		logger.Info("cocoded listening", "addr", config.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("cocoded server failed", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("cocoded shutdown failed", "error", err)
		os.Exit(1)
	}
	logger.Info("cocoded stopped")
}
