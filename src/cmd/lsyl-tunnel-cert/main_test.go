package main

import (
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"
)

func TestGenerateServerCertVerifiesForHost(t *testing.T) {
	certPEM, keyPEM, err := generateServerCert([]string{"localhost", "127.0.0.1"}, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(keyPEM) == 0 {
		t.Fatal("missing private key")
	}
	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatal("decode cert PEM failed")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	if _, err := cert.Verify(x509.VerifyOptions{DNSName: "localhost", Roots: pool}); err != nil {
		t.Fatal(err)
	}
}
