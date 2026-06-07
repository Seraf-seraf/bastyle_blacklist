package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/app"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/pkg/logging"
)

func main() {
	const methodCtx = "cmd/main"

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	configPath := flag.String("config", "config/config.yaml", "путь к YAML-конфигу")
	flag.Parse()

	shutdown, err := app.Run(ctx, *configPath)
	if err != nil {
		logging.Panic(methodCtx, err)
	}

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := shutdown(shutdownCtx); err != nil {
		logging.Panic(methodCtx, err)
	}
}
