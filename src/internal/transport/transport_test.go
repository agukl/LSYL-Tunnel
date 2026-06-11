package transport

import (
	"bytes"
	"io"
	"net"
	"testing"
	"time"
)

func TestProxyPairWithOptionsRateLimitsAggregateStream(t *testing.T) {
	leftProxy, leftPeer := net.Pipe()
	rightProxy, rightPeer := net.Pipe()
	defer leftPeer.Close()
	defer rightPeer.Close()

	done := make(chan struct{})
	go func() {
		ProxyPairWithOptions(leftProxy, rightProxy, nil, nil, ProxyOptions{RateLimitBytesPerSec: 8 * 1024})
		close(done)
	}()

	payload := bytes.Repeat([]byte("x"), 4*1024)
	writeDone := make(chan error, 1)
	started := time.Now()
	go func() {
		_, err := leftPeer.Write(payload)
		_ = leftPeer.Close()
		writeDone <- err
	}()

	got := make([]byte, len(payload))
	if _, err := io.ReadFull(rightPeer, got); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(started)
	if elapsed < 250*time.Millisecond {
		t.Fatalf("rate limit did not delay transfer enough: %s", elapsed)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("payload changed during proxy")
	}
	if err := <-writeDone; err != nil {
		t.Fatal(err)
	}
	_ = rightPeer.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("proxy did not stop")
	}
}
