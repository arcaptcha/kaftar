package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/arcaptcha/kaftar/server/internal/api/http"
	"github.com/arcaptcha/kaftar/server/internal/app"
	"github.com/arcaptcha/kaftar/server/internal/config"
	"github.com/arcaptcha/kaftar/server/pkg/logger"
	"github.com/joho/godotenv"
	"go.uber.org/zap"
)

var envFile = flag.String("env", "", "path to .env file")

func main() {
	flag.Parse()

	if *envFile != "" {
		if err := godotenv.Load(*envFile); err != nil {
			panic(err)
		}
	}

	cfg := config.MustReadEnv()

	// initial logger
	loggerMode := logger.ModeProduction
	if cfg.DevMode {
		loggerMode = logger.ModeDevelopment
	}
	logger := logger.NewZapLogger(loggerMode)
	defer logger.Sync()

	// instantiate http server
	theApp := app.New(cfg, logger)
	server := http.NewServer(theApp)

	// run server
	go func() {
		if err := server.Start(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("server failed to running", zap.Error(err))
		}
	}()

	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)
	<-done

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Fatal("server failed to shutdown")
	}
}
