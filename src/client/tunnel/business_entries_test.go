package tunnel

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestBusinessEndpointPoolSelectsEntryAndFallsBack(t *testing.T) {
	bootstrap, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer bootstrap.Close()
	entry, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	pool := newBusinessEndpointPool([]string{entry.Addr().String()}, bootstrap.Addr().String(), nil)
	dialer := &businessPoolDialer{pool: pool, timeout: time.Second}
	conn, err := dialer.DialContext(context.Background(), "tcp", bootstrap.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if got := conn.RemoteAddr().String(); got != entry.Addr().String() {
		t.Fatalf("selected %s, want pool entry %s", got, entry.Addr())
	}
	_ = conn.Close()
	if quality := pool.qualitySnapshot(); len(quality) != 2 || quality[0].Attempts != 0 || quality[1].Successes != 1 {
		t.Fatalf("quality after pool dial = %+v", quality)
	}
	_ = entry.Close()
	conn, err = dialer.DialContext(context.Background(), "tcp", bootstrap.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if got := conn.RemoteAddr().String(); got != bootstrap.Addr().String() {
		t.Fatalf("selected %s, want bootstrap fallback %s", got, bootstrap.Addr())
	}
	_ = conn.Close()
	if quality := pool.qualitySnapshot(); quality[0].Successes != 1 || quality[1].Failures != 1 {
		t.Fatalf("quality after fallback = %+v", quality)
	}
}

func TestBusinessEndpointPoolAcceptsIPv6Literal(t *testing.T) {
	pool := newBusinessEndpointPool([]string{"[2001:db8::10]:50000", "[::]:50001", "invalid"}, "127.0.0.1:3443", nil)
	if len(pool.entries) != 1 || pool.entries[0] != "[2001:db8::10]:50000" {
		t.Fatalf("entries = %v", pool.entries)
	}
}

func TestBusinessEndpointPoolSkipsVirtualRedirectPort(t *testing.T) {
	forwards := []ForwardConfig{{Direction: DirectionVirtual, ListenAddr: ":50000"}}
	pool := newBusinessEndpointPool([]string{"203.0.113.10:50000", "203.0.113.10:50001"}, "203.0.113.10:3443", forwards)
	if len(pool.entries) != 1 || pool.entries[0] != "203.0.113.10:50001" {
		t.Fatalf("entries = %v", pool.entries)
	}
}
