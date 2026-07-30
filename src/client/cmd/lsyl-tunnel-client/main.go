package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"lsyltunnel/src/client/tunnel"
)

func main() {
	if handled, err := tunnel.HandleVirtualRedirectHelperArgs(os.Args[1:]); handled {
		if err != nil {
			log.Print(err)
			os.Exit(1)
		}
		return
	}
	configPath := flag.String("config", "src/client/conf/client.yaml", "client config file")
	flag.Parse()
	cfg, err := tunnel.LoadConfig(*configPath)
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	logf := func(format string, args ...any) { log.Printf(format, args...) }
	resp, err := tunnel.CheckLoginResponse(ctx, cfg)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		log.Fatal(err)
	}
	if ctx.Err() != nil {
		return
	}
	client, err := tunnel.StartVerified(ctx, cfg, resp.ServerVersion, logf)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		log.Fatal(err)
	}
	defer client.Close()
	<-ctx.Done()
}
