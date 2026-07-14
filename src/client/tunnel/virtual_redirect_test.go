package tunnel

import (
	"encoding/binary"
	"net"
	"strings"
	"testing"
)

func TestBuildVirtualRedirectFilterMatchesOnlyConfiguredEndpoints(t *testing.T) {
	rules := []virtualRedirectRule{
		{VirtualIP: "203.0.113.10", VirtualPort: 22, LocalPort: 45000},
		{VirtualIP: "203.0.113.10", VirtualPort: 443, LocalPort: 45001},
	}
	filter, err := buildVirtualRedirectFilter(rules)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{
		"ip.DstAddr == 203.0.113.10 and tcp.DstPort == 22",
		"ip.DstAddr == 203.0.113.10 and tcp.DstPort == 443",
		"ip.DstAddr == 203.0.113.10 and tcp.SrcPort == 45000",
		"inbound and not impostor",
	} {
		if !strings.Contains(filter, value) {
			t.Fatalf("filter %q does not contain %q", filter, value)
		}
	}
	if strings.Contains(filter, "3443") {
		t.Fatalf("filter unexpectedly contains the tunnel port: %s", filter)
	}
}

func TestNormalizeVirtualRedirectRulesEnforcesFilterLimit(t *testing.T) {
	rules := make([]virtualRedirectRule, maxVirtualRedirectEndpoints+1)
	for i := range rules {
		rules[i] = virtualRedirectRule{
			VirtualIP:   "203.0.113.10",
			VirtualPort: uint16(10000 + i),
			LocalPort:   uint16(20000 + i),
		}
	}
	if _, err := normalizeVirtualRedirectRules(rules); err == nil || !strings.Contains(err.Error(), "at most 48 endpoints") {
		t.Fatalf("normalizeVirtualRedirectRules() error = %v, want endpoint limit", err)
	}
}

func TestRewriteVirtualRedirectPacketReflectsTargetAndResponse(t *testing.T) {
	rule := virtualRedirectRule{VirtualIP: "203.0.113.10", VirtualPort: 22, LocalPort: 45000}
	table, err := newVirtualRedirectTable([]virtualRedirectRule{rule})
	if err != nil {
		t.Fatal(err)
	}

	targetPacket := testIPv4TCPPacket("192.168.10.20", 51000, rule.VirtualIP, rule.VirtualPort)
	outbound, action := rewriteVirtualRedirectPacket(targetPacket, true, table)
	if outbound || action != virtualRedirectInject {
		t.Fatalf("target rewrite = outbound %v action %v", outbound, action)
	}
	assertTestIPv4TCPPacket(t, targetPacket, rule.VirtualIP, 51000, "192.168.10.20", rule.LocalPort)

	responsePacket := testIPv4TCPPacket("192.168.10.20", rule.LocalPort, rule.VirtualIP, 51000)
	outbound, action = rewriteVirtualRedirectPacket(responsePacket, true, table)
	if outbound || action != virtualRedirectInject {
		t.Fatalf("response rewrite = outbound %v action %v", outbound, action)
	}
	assertTestIPv4TCPPacket(t, responsePacket, rule.VirtualIP, rule.VirtualPort, "192.168.10.20", 51000)
}

func TestRewriteVirtualRedirectPacketDropsExternalAccessToLocalPort(t *testing.T) {
	rule := virtualRedirectRule{VirtualIP: "203.0.113.10", VirtualPort: 22, LocalPort: 45000}
	table, err := newVirtualRedirectTable([]virtualRedirectRule{rule})
	if err != nil {
		t.Fatal(err)
	}
	packet := testIPv4TCPPacket("198.51.100.30", 51000, "192.168.10.20", rule.LocalPort)
	_, action := rewriteVirtualRedirectPacket(packet, false, table)
	if action != virtualRedirectDrop {
		t.Fatalf("inbound local redirect action = %v", action)
	}
}

func TestRewriteVirtualRedirectPacketPassesUnrelatedTunnelPort(t *testing.T) {
	rule := virtualRedirectRule{VirtualIP: "203.0.113.10", VirtualPort: 22, LocalPort: 45000}
	table, err := newVirtualRedirectTable([]virtualRedirectRule{rule})
	if err != nil {
		t.Fatal(err)
	}
	packet := testIPv4TCPPacket("192.168.10.20", 51000, rule.VirtualIP, 3443)
	outbound, action := rewriteVirtualRedirectPacket(packet, true, table)
	if !outbound || action != virtualRedirectPass {
		t.Fatalf("tunnel packet rewrite = outbound %v action %v", outbound, action)
	}
}

func testIPv4TCPPacket(srcIP string, srcPort uint16, dstIP string, dstPort uint16) []byte {
	packet := make([]byte, 40)
	packet[0] = 0x45
	packet[9] = 6
	copy(packet[12:16], net.ParseIP(srcIP).To4())
	copy(packet[16:20], net.ParseIP(dstIP).To4())
	binary.BigEndian.PutUint16(packet[20:22], srcPort)
	binary.BigEndian.PutUint16(packet[22:24], dstPort)
	return packet
}

func assertTestIPv4TCPPacket(t *testing.T, packet []byte, srcIP string, srcPort uint16, dstIP string, dstPort uint16) {
	t.Helper()
	if got := net.IP(packet[12:16]).String(); got != srcIP {
		t.Fatalf("source IP = %s, want %s", got, srcIP)
	}
	if got := binary.BigEndian.Uint16(packet[20:22]); got != srcPort {
		t.Fatalf("source port = %d, want %d", got, srcPort)
	}
	if got := net.IP(packet[16:20]).String(); got != dstIP {
		t.Fatalf("destination IP = %s, want %s", got, dstIP)
	}
	if got := binary.BigEndian.Uint16(packet[22:24]); got != dstPort {
		t.Fatalf("destination port = %d, want %d", got, dstPort)
	}
}
