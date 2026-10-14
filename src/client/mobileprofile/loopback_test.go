package mobileprofile

import (
	"net"
	"strings"
	"testing"
)

func TestProfileImportAndExportRejectLoopbackAliases(t *testing.T) {
	cert := validImportCert(t)
	for _, host := range []string{"localhost", "::1", "127.0.0.2", "::ffff:127.0.0.1"} {
		for _, field := range []string{"server_addr", "tls.server_name", "listen_addr", "server_target"} {
			t.Run(host+"/"+field, func(t *testing.T) {
				profile := validImportProfile(t)
				switch field {
				case "server_addr":
					profile[field] = net.JoinHostPort(host, "3443")
				case "tls.server_name":
					profile["tls"].(map[string]any)["server_name"] = host
				default:
					profile["forwards"].([]map[string]any)[0][field] = net.JoinHostPort(host, "13389")
				}
				_, err := ImportBytes(profileZip(t, profile, cert))
				if err == nil || !strings.Contains(err.Error(), "127.0.0.1") {
					t.Fatalf("import: %v", err)
				}

				valid, err := ImportBytes(profileZip(t, validImportProfile(t), cert))
				if err != nil {
					t.Fatal(err)
				}
				cfg := valid.Config
				switch field {
				case "server_addr":
					cfg.ServerAddr = net.JoinHostPort(host, "3443")
				case "tls.server_name":
					cfg.TLS.ServerName = host
				case "listen_addr":
					cfg.Forwards[0].ListenAddr = net.JoinHostPort(host, "13389")
				case "server_target":
					cfg.Forwards[0].ServerTarget = net.JoinHostPort(host, "3389")
				}
				_, _, err = ProfileFromConfig(cfg)
				if err == nil || !strings.Contains(err.Error(), "127.0.0.1") {
					t.Fatalf("export: %v", err)
				}
			})
		}
	}
}
