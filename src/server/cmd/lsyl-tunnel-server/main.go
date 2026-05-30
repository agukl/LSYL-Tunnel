package main

import (
	"context"
	"flag"
	"log"
	"os/signal"
	"syscall"

	"lsyltunnel/src/server/tunnel"
)

func main() {
	configPath := flag.String("config", "src/server/conf/server.yaml", "server config file")
	flag.Parse()
	cfg, err := tunnel.LoadConfig(*configPath)
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	logf := func(format string, args ...any) { log.Printf(format, args...) }
	if err := tunnel.Run(ctx, cfg, logf); err != nil && ctx.Err() == nil {
		log.Fatal(err)
	}
}
