package transport

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

const (
	certificateGateVersion       = byte(1)
	certificateGateNonceSize     = 32
	certificateGateChallengeSize = 8 + 1 + certificateGateNonceSize
	certificateGateResponseSize  = 8 + 1 + certificateGateNonceSize + sha256.Size
)

var (
	certificateGateChallengeMagic = []byte("LSYLGATE")
	certificateGateResponseMagic  = []byte("LSYLPROF")
	certificateGateKeyContext     = []byte("LSYL-TUNNEL-CERTIFICATE-GATE-KEY-V1\x00")
	certificateGateProofContext   = []byte("LSYL-TUNNEL-CERTIFICATE-GATE-PROOF-V1\x00")

	ErrCertificateGateProtocol = errors.New("invalid certificate possession gate protocol")
	ErrCertificateGateProof    = errors.New("certificate possession gate proof rejected")
)

type CertificateGateKey [sha256.Size]byte

// CertificateGateKeyFromDER derives the shared pre-TLS gate material from the
// exact certificate bytes distributed to clients. It is an admission token,
// not a client identity or a replacement for the TLS handshake that follows.
func CertificateGateKeyFromDER(der []byte) (CertificateGateKey, error) {
	var key CertificateGateKey
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return key, fmt.Errorf("parse certificate possession gate certificate: %w", err)
	}
	hash := sha256.New()
	_, _ = hash.Write(certificateGateKeyContext)
	_, _ = hash.Write(cert.Raw)
	copy(key[:], hash.Sum(nil))
	return key, nil
}

func CertificateGateKeyFromPEM(data []byte) (CertificateGateKey, error) {
	var key CertificateGateKey
	for len(data) > 0 {
		block, rest := pem.Decode(data)
		if block == nil {
			break
		}
		data = rest
		if block.Type == "CERTIFICATE" {
			return CertificateGateKeyFromDER(block.Bytes)
		}
	}
	return key, fmt.Errorf("certificate possession gate requires an X.509 certificate")
}

func ServerCertificateGate(conn net.Conn, key CertificateGateKey) error {
	if conn == nil {
		return fmt.Errorf("%w: connection is nil", ErrCertificateGateProtocol)
	}
	challenge := make([]byte, certificateGateChallengeSize)
	copy(challenge, certificateGateChallengeMagic)
	challenge[len(certificateGateChallengeMagic)] = certificateGateVersion
	if _, err := rand.Read(challenge[9:]); err != nil {
		return fmt.Errorf("generate certificate possession gate challenge: %w", err)
	}
	if err := writeAll(conn, challenge); err != nil {
		return fmt.Errorf("write certificate possession gate challenge: %w", err)
	}

	response := make([]byte, certificateGateResponseSize)
	if _, err := io.ReadFull(conn, response); err != nil {
		return fmt.Errorf("read certificate possession gate proof: %w", err)
	}
	if !bytes.Equal(response[:8], certificateGateResponseMagic) || response[8] != certificateGateVersion {
		return ErrCertificateGateProtocol
	}
	expected := certificateGateProof(key, challenge, response[:9+certificateGateNonceSize])
	if !hmac.Equal(response[9+certificateGateNonceSize:], expected) {
		return ErrCertificateGateProof
	}
	return nil
}

// ClientCertificateGate completes the fixed-size challenge/proof exchange.
// The server sends no TLS handshake bytes until ServerCertificateGate accepts.
func ClientCertificateGate(conn net.Conn, key CertificateGateKey) error {
	if conn == nil {
		return fmt.Errorf("%w: connection is nil", ErrCertificateGateProtocol)
	}
	challenge := make([]byte, certificateGateChallengeSize)
	if _, err := io.ReadFull(conn, challenge); err != nil {
		return fmt.Errorf("read certificate possession gate challenge: %w", err)
	}
	if !bytes.Equal(challenge[:8], certificateGateChallengeMagic) || challenge[8] != certificateGateVersion {
		return ErrCertificateGateProtocol
	}

	response := make([]byte, certificateGateResponseSize)
	copy(response, certificateGateResponseMagic)
	response[8] = certificateGateVersion
	if _, err := rand.Read(response[9 : 9+certificateGateNonceSize]); err != nil {
		return fmt.Errorf("generate certificate possession gate nonce: %w", err)
	}
	proof := certificateGateProof(key, challenge, response[:9+certificateGateNonceSize])
	copy(response[9+certificateGateNonceSize:], proof)
	if err := writeAll(conn, response); err != nil {
		return fmt.Errorf("write certificate possession gate proof: %w", err)
	}
	return nil
}

func DialTLSWithCertificateGate(
	ctx context.Context,
	dialer *net.Dialer,
	network string,
	address string,
	tlsConfig *tls.Config,
	gateKey CertificateGateKey,
	handshakeTimeout time.Duration,
) (*tls.Conn, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if dialer == nil {
		dialer = &net.Dialer{}
	}
	raw, err := dialer.DialContext(ctx, network, address)
	if err != nil {
		return nil, err
	}
	closeOnError := true
	defer func() {
		if closeOnError {
			_ = raw.Close()
		}
	}()

	if handshakeTimeout <= 0 {
		handshakeTimeout = 5 * time.Second
	}
	deadline := time.Now().Add(handshakeTimeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	_ = raw.SetDeadline(deadline)
	if err := ClientCertificateGate(raw, gateKey); err != nil {
		return nil, err
	}
	tlsConn := tls.Client(raw, tlsConfig)
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		return nil, fmt.Errorf("TLS handshake after certificate possession gate: %w", err)
	}
	_ = tlsConn.SetDeadline(time.Time{})
	closeOnError = false
	return tlsConn, nil
}

func certificateGateProof(key CertificateGateKey, challenge, responseHeader []byte) []byte {
	mac := hmac.New(sha256.New, key[:])
	_, _ = mac.Write(certificateGateProofContext)
	_, _ = mac.Write(challenge)
	_, _ = mac.Write(responseHeader)
	return mac.Sum(nil)
}

func writeAll(writer io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := writer.Write(data)
		if err != nil {
			return err
		}
		if n <= 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}
