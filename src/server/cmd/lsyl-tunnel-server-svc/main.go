package main

import (
	"context"
	"flag"
	"log"
	"path/filepath"

	"lsyltunnel/src/internal/winservice"
	"lsyltunnel/src/server/tunnel"
)

func main() {
	configPath := flag.String("config", filepath.Join("server", "conf", "server.yaml"), "server config file")
	logPath := flag.String("log", filepath.Join("logs", "server-service.log"), "service log file")
	serviceName := flag.String("service-name", "LSYLTunnelServer", "Windows service name")
	flag.Parse()
	err := winservice.Run(winservice.Options{
		Name:    *serviceName,
		LogFile: *logPath,
		Run: func(ctx context.Context, logf func(string, ...any)) error {
			cfg, err := tunnel.LoadConfig(*configPath)
			if err != nil {
				logf("load config failed: %v", err)
				return err
			}
			return tunnel.Run(ctx, cfg, logf)
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}
