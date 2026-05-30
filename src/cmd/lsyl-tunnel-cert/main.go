package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"flag"
	"fmt"
	"log"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	outDir := flag.String("out", "certs", "output directory for server TLS identity files")
	hosts := flag.String("hosts", "localhost,127.0.0.1", "comma-separated server DNS names or IP addresses")
	days := flag.Int("days", 825, "server TLS identity validity in days")
	flag.Parse()
	if *days <= 0 {
		log.Fatal("days must be greater than zero")
	}
	certPEM, keyPEM, err := generateServerCert(strings.Split(*hosts, ","), time.Duration(*days)*24*time.Hour)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		log.Fatal(err)
	}
	certFile := filepath.Join(*outDir, "server.crt")
	keyFile := filepath.Join(*outDir, "server.key")
	if err := os.WriteFile(certFile, certPEM, 0o644); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(keyFile, keyPEM, 0o600); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("wrote %s\n", certFile)
	fmt.Printf("wrote %s\n", keyFile)
}

func generateServerCert(hosts []string, validity time.Duration) ([]byte, []byte, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "lsyl-tunnel-server"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(validity),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	for _, host := range hosts {
		host = strings.TrimSpace(host)
		if host == "" {
			continue
		}
		if ip := net.ParseIP(host); ip != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
		} else {
			tmpl.DNSNames = append(tmpl.DNSNames, host)
		}
	}
	if len(tmpl.DNSNames) == 0 && len(tmpl.IPAddresses) == 0 {
		return nil, nil, fmt.Errorf("at least one host is required")
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		return nil, nil, err
	}
	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return nil, nil, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM, nil
}
