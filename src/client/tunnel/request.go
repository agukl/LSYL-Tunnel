package tunnel

import (
	"context"
	"crypto/tls"
	"net"
	"time"

	"lsyltunnel/src/internal/protocol"
	"lsyltunnel/src/internal/transport"
	appversion "lsyltunnel/src/internal/version"
)

func CheckLogin(ctx context.Context, cfg Config) error {
	_, err := CheckLoginResponse(ctx, cfg)
	return err
}

func CheckLoginResponse(ctx context.Context, cfg Config) (protocol.OpenResponse, error) {
	return checkServerResponse(ctx, cfg, "login")
}

func CheckHealthResponse(ctx context.Context, cfg Config) (protocol.OpenResponse, error) {
	return checkServerResponse(ctx, cfg, "health")
}

func newOpenRequest(cfg Config, reqType string) protocol.OpenRequest {
	return protocol.OpenRequest{
		Type:            reqType,
		Username:        cfg.Username,
		Password:        cfg.Password,
		Credential:      credentialFromConfig(cfg),
		ClientID:        cfg.ClientID,
		ClientVersion:   appversion.AppVersion,
		ProtocolVersion: appversion.ProtocolVersion,
	}
}

func checkServerResponse(ctx context.Context, cfg Config, reqType string) (protocol.OpenResponse, error) {
	ApplyDefaults(&cfg)
	tlsCfg, certificateGateKey, err := clientTLSMaterials(cfg)
	if err != nil {
		return protocol.OpenResponse{}, err
	}
	return checkServerResponseWithTLS(ctx, cfg, reqType, tlsCfg, certificateGateKey)
}

func checkServerResponseWithTLS(ctx context.Context, cfg Config, reqType string, tlsCfg *tls.Config, certificateGateKey transport.CertificateGateKey) (protocol.OpenResponse, error) {
	var resp protocol.OpenResponse
	timeout := time.Duration(cfg.Connection.DialTimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	conn, err := transport.DialTLSWithCertificateGate(
		ctx,
		&net.Dialer{Timeout: timeout},
		"tcp",
		cfg.ServerAddr,
		tlsCfg,
		certificateGateKey,
		timeout,
	)
	if err != nil {
		return resp, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))
	req := newOpenRequest(cfg, reqType)
	if err := protocol.WriteJSON(conn, req); err != nil {
		return resp, err
	}
	if err := protocol.ReadJSON(conn, &resp, protocol.DefaultMaxHandshakeBytes); err != nil {
		return resp, err
	}
	if !resp.OK {
		if resp.Message == "" {
			resp.Message = reqType + " failed"
		}
		return resp, responseError(resp, reqType+" failed")
	}
	return resp, nil
}
