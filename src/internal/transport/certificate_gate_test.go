package transport

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net"
	"testing"
	"time"
)

func TestCertificateGateAcceptsMatchingCertificate(t *testing.T) {
	certPEM, certDER := certificateGateTestCertificate(t)
	clientKey, err := CertificateGateKeyFromPEM(certPEM)
	if err != nil {
		t.Fatal(err)
	}
	serverKey, err := CertificateGateKeyFromDER(certDER)
	if err != nil {
		t.Fatal(err)
	}

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()
	serverResult := make(chan error, 1)
	go func() { serverResult <- ServerCertificateGate(serverConn, serverKey) }()
	if err := ClientCertificateGate(clientConn, clientKey); err != nil {
		t.Fatal(err)
	}
	if err := <-serverResult; err != nil {
		t.Fatal(err)
	}
}

func TestCertificateGateRejectsDifferentCertificate(t *testing.T) {
	_, serverDER := certificateGateTestCertificate(t)
	clientPEM, _ := certificateGateTestCertificate(t)
	serverKey, err := CertificateGateKeyFromDER(serverDER)
	if err != nil {
		t.Fatal(err)
	}
	clientKey, err := CertificateGateKeyFromPEM(clientPEM)
	if err != nil {
		t.Fatal(err)
	}

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()
	serverResult := make(chan error, 1)
	go func() { serverResult <- ServerCertificateGate(serverConn, serverKey) }()
	if err := ClientCertificateGate(clientConn, clientKey); err != nil {
		t.Fatal(err)
	}
	if err := <-serverResult; !errors.Is(err, ErrCertificateGateProof) {
		t.Fatalf("server error = %v, want ErrCertificateGateProof", err)
	}
}

func TestCertificateGateRejectsMalformedResponse(t *testing.T) {
	_, serverDER := certificateGateTestCertificate(t)
	serverKey, err := CertificateGateKeyFromDER(serverDER)
	if err != nil {
		t.Fatal(err)
	}

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()
	deadline := time.Now().Add(time.Second)
	_ = serverConn.SetDeadline(deadline)
	_ = clientConn.SetDeadline(deadline)
	serverResult := make(chan error, 1)
	go func() { serverResult <- ServerCertificateGate(serverConn, serverKey) }()

	challenge := make([]byte, certificateGateChallengeSize)
	if _, err := io.ReadFull(clientConn, challenge); err != nil {
		t.Fatal(err)
	}
	malformed := make([]byte, certificateGateResponseSize)
	copy(malformed, []byte("NOTGATE!"))
	if err := writeAll(clientConn, malformed); err != nil {
		t.Fatal(err)
	}
	if err := <-serverResult; !errors.Is(err, ErrCertificateGateProtocol) {
		t.Fatalf("server error = %v, want ErrCertificateGateProtocol", err)
	}
}

func TestCertificateGateHonorsConnectionDeadline(t *testing.T) {
	_, serverDER := certificateGateTestCertificate(t)
	serverKey, err := CertificateGateKeyFromDER(serverDER)
	if err != nil {
		t.Fatal(err)
	}

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()
	_ = serverConn.SetDeadline(time.Now().Add(100 * time.Millisecond))
	challengeRead := make(chan error, 1)
	go func() {
		_, err := io.ReadFull(clientConn, make([]byte, certificateGateChallengeSize))
		challengeRead <- err
	}()

	err = ServerCertificateGate(serverConn, serverKey)
	if readErr := <-challengeRead; readErr != nil {
		t.Fatal(readErr)
	}
	var netErr net.Error
	if !errors.As(err, &netErr) || !netErr.Timeout() {
		t.Fatalf("server error = %v, want timeout", err)
	}
}

func TestCertificateGateKeyUsesCertificateDER(t *testing.T) {
	certPEM, certDER := certificateGateTestCertificate(t)
	padded := append([]byte("ignored text\n"), certPEM...)
	padded = append(padded, []byte("\nignored tail\n")...)
	fromPEM, err := CertificateGateKeyFromPEM(padded)
	if err != nil {
		t.Fatal(err)
	}
	fromDER, err := CertificateGateKeyFromDER(certDER)
	if err != nil {
		t.Fatal(err)
	}
	if fromPEM != fromDER {
		t.Fatal("PEM and DER derived different certificate gate keys")
	}
}

func TestCertificateGateProofMatchesAndroidWireVector(t *testing.T) {
	keyBytes, err := hex.DecodeString("f829879b7c1a96a66fa8674bfbbb229aca39de0132469a3b87bae52b0d94307e")
	if err != nil {
		t.Fatal(err)
	}
	var key CertificateGateKey
	copy(key[:], keyBytes)
	challenge := make([]byte, certificateGateChallengeSize)
	copy(challenge, certificateGateChallengeMagic)
	challenge[8] = certificateGateVersion
	for i := 0; i < certificateGateNonceSize; i++ {
		challenge[9+i] = byte(i)
	}
	responseHeader := make([]byte, 9+certificateGateNonceSize)
	copy(responseHeader, certificateGateResponseMagic)
	responseHeader[8] = certificateGateVersion
	for i := 0; i < certificateGateNonceSize; i++ {
		responseHeader[9+i] = byte(i + 32)
	}
	want, err := hex.DecodeString("174c4c8b670c9fb3c6c236000e363debe79023bb7d7001d6cd1c9aad8715f329")
	if err != nil {
		t.Fatal(err)
	}
	if got := certificateGateProof(key, challenge, responseHeader); !hmac.Equal(got, want) {
		t.Fatalf("proof = %x, want %x", got, want)
	}
}

func TestDialTLSWithCertificateGateInfersServerName(t *testing.T) {
	certPEM, keyPEM, certDER := certificateGateTestIdentity(t)
	identity, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	gateKey, err := CertificateGateKeyFromDER(certDER)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	serverResult := make(chan error, 1)
	go func() {
		raw, err := listener.Accept()
		if err != nil {
			serverResult <- err
			return
		}
		defer raw.Close()
		_ = raw.SetDeadline(time.Now().Add(2 * time.Second))
		if err := ServerCertificateGate(raw, gateKey); err != nil {
			serverResult <- err
			return
		}
		server := tls.Server(raw, &tls.Config{
			Certificates: []tls.Certificate{identity},
			MinVersion:   tls.VersionTLS12,
		})
		serverResult <- server.Handshake()
	}()

	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(certPEM) {
		t.Fatal("append test root certificate")
	}
	tlsConfig := &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
	client, err := DialTLSWithCertificateGate(
		context.Background(),
		&net.Dialer{Timeout: time.Second},
		"tcp4",
		listener.Addr().String(),
		tlsConfig,
		gateKey,
		2*time.Second,
	)
	if err != nil {
		t.Fatal(err)
	}
	_ = client.Close()
	if err := <-serverResult; err != nil {
		t.Fatal(err)
	}
	if tlsConfig.ServerName != "" {
		t.Fatalf("DialTLSWithCertificateGate mutated cached ServerName to %q", tlsConfig.ServerName)
	}
}

func certificateGateTestCertificate(t *testing.T) ([]byte, []byte) {
	t.Helper()
	certPEM, _, certDER := certificateGateTestIdentity(t)
	return certPEM, certDER
}

func certificateGateTestIdentity(t *testing.T) ([]byte, []byte, []byte) {
	t.Helper()
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "certificate-gate-test"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	privateDER, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privateDER}),
		der
}
