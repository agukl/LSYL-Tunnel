package tunnel

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"lsyltunnel/src/internal/transport"
)

func (c *Client) clientTLSConfig() (*tls.Config, error) {
	if c != nil && c.tlsConfig != nil {
		return c.tlsConfig, nil
	}
	return clientTLSConfig(c.cfg)
}

func clientTLSConfig(cfg Config) (*tls.Config, error) {
	tlsCfg, _, err := clientTLSMaterials(cfg)
	return tlsCfg, err
}

func clientTLSMaterials(cfg Config) (*tls.Config, transport.CertificateGateKey, error) {
	var certificateGateKey transport.CertificateGateKey
	tlsCfg := &tls.Config{
		MinVersion:         transport.TLSMinVersion(cfg.TLS.MinVersion),
		ServerName:         cfg.TLS.ServerName,
		InsecureSkipVerify: cfg.TLS.InsecureSkipVerify,
	}
	if cfg.TLS.CACertFile == "" {
		return nil, certificateGateKey, fmt.Errorf("tls.ca_cert_file is required for certificate possession gate and server verification")
	}
	pemData, err := os.ReadFile(cfg.TLS.CACertFile)
	if err != nil {
		return nil, certificateGateKey, err
	}
	certificateGateKey, err = transport.CertificateGateKeyFromPEM(pemData)
	if err != nil {
		return nil, certificateGateKey, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pemData) {
		return nil, certificateGateKey, fmt.Errorf("no server TLS trust data found in %s", cfg.TLS.CACertFile)
	}
	tlsCfg.RootCAs = pool
	return tlsCfg, certificateGateKey, nil
}
