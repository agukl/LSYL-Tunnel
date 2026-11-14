package tunnel

import (
	"io"
	"net"
	"sync/atomic"
	"testing"
)

func TestMeteredBusinessConnCountsTransferredBytes(t *testing.T) {
	local, remote := net.Pipe()
	defer local.Close()
	defer remote.Close()
	var up, down atomic.Int64
	metered := &meteredBusinessConn{Conn: local, up: &up, down: &down}

	written := make(chan error, 1)
	go func() { _, err := remote.Write([]byte("down")); written <- err }()
	buf := make([]byte, 4)
	if _, err := io.ReadFull(metered, buf); err != nil || string(buf) != "down" {
		t.Fatalf("read = %q, %v", buf, err)
	}
	if err := <-written; err != nil {
		t.Fatal(err)
	}

	read := make(chan error, 1)
	go func() { _, err := io.ReadFull(remote, buf); read <- err }()
	if _, err := metered.Write([]byte("up!!")); err != nil {
		t.Fatal(err)
	}
	if err := <-read; err != nil {
		t.Fatal(err)
	}
	if up.Load() != 4 || down.Load() != 4 {
		t.Fatalf("bytes up/down = %d/%d, want 4/4", up.Load(), down.Load())
	}
}
