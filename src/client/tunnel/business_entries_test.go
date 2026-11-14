package tunnel

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestBusinessEndpointPoolFallsBackToBootstrap(t *testing.T) {
	bootstrap, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer bootstrap.Close()
	pool := newBusinessEndpointPool([]string{"[2001:db8::10]:50000"}, bootstrap.Addr().String())
	dialer := &businessPoolDialer{pool: pool, timeout: time.Second}
	conn, err := dialer.DialContext(context.Background(), "tcp", bootstrap.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if got := conn.RemoteAddr().String(); got != bootstrap.Addr().String() {
		t.Fatalf("selected %s, want bootstrap fallback %s", got, bootstrap.Addr())
	}
	_ = conn.Close()
	if pool.quality[pool.entries[0]].attempts != 1 {
		t.Fatalf("quality after fallback = %+v", pool.quality)
	}
}

func TestBusinessDialerWithoutIPv6PoolUsesBootstrap(t *testing.T) {
	bootstrap, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer bootstrap.Close()
	client := &Client{businessPool: newBusinessEndpointPool(nil, bootstrap.Addr().String())}
	conn, err := client.businessDialer(time.Second).DialContext(context.Background(), "tcp", bootstrap.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if got := conn.RemoteAddr().String(); got != bootstrap.Addr().String() {
		t.Fatalf("selected %s, want bootstrap %s", got, bootstrap.Addr())
	}
}

func TestBusinessEndpointPoolOnlyAcceptsGlobalIPv6(t *testing.T) {
	pool := newBusinessEndpointPool([]string{
		"[2001:db8::10]:50000", "[2001:db8::10]:50000", "[::]:50001", "[::1]:50001",
		"[fe80::1]:50001", "[fd00::1]:50001", "[::ffff:192.0.2.1]:50001",
		"203.0.113.10:50000", ":50000", "invalid",
	}, "127.0.0.1:3443")
	if len(pool.entries) != 1 || pool.entries[0] != "[2001:db8::10]:50000" {
		t.Fatalf("entries = %v", pool.entries)
	}
}

func TestBusinessEndpointPoolIgnoresOldIPv4Candidates(t *testing.T) {
	pool := newBusinessEndpointPool([]string{":50000", "203.0.113.10:50000"}, "203.0.113.10:3443")
	if len(pool.entries) != 0 {
		t.Fatalf("old IPv4 entries accepted: %v", pool.entries)
	}
}

func TestBusinessEndpointPoolSamplesThenPrefersQuality(t *testing.T) {
	pool := newBusinessEndpointPool([]string{
		"[2001:db8::1]:3443", "[2001:db8::2]:3443", "[2001:db8::3]:3443",
	}, "127.0.0.1:3443")
	pool.quality[pool.entries[0]] = endpointDialQuality{attempts: 2, successes: 2, totalDialMS: 80}
	pool.quality[pool.entries[1]] = endpointDialQuality{attempts: 2, successes: 2, totalDialMS: 20}
	ordered := pool.orderedEntries()
	if ordered[0] != pool.entries[2] || ordered[1] != pool.entries[1] || ordered[2] != pool.entries[0] {
		t.Fatalf("sampling order = %v", ordered)
	}
	pool.quality[pool.entries[2]] = endpointDialQuality{attempts: 2, successes: 1, totalDialMS: 5}
	ordered = pool.orderedEntries()
	if ordered[0] != pool.entries[1] || ordered[1] != pool.entries[0] || ordered[2] != pool.entries[2] {
		t.Fatalf("quality order = %v", ordered)
	}
}
