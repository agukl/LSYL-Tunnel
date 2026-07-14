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
		"loopback and ip.SrcAddr == 127.0.0.1 and ip.DstAddr == 127.0.0.1 and tcp.SrcPort == 45000",
		"ip and tcp and outbound",
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
	route, action := rewriteVirtualRedirectPacket(targetPacket, virtualRedirectPacketRoute{Outbound: true, IfIdx: 7, SubIfIdx: 2}, &table)
	if !route.Outbound || !route.Loopback || route.IfIdx != 1 || action != virtualRedirectInject {
		t.Fatalf("target rewrite = route %+v action %v", route, action)
	}
	assertTestIPv4TCPPacket(t, targetPacket, "127.0.0.1", 51000, "127.0.0.1", rule.LocalPort)

	responsePacket := testIPv4TCPPacket("127.0.0.1", rule.LocalPort, "127.0.0.1", 51000)
	route, action = rewriteVirtualRedirectPacket(responsePacket, virtualRedirectPacketRoute{Outbound: true, Loopback: true, IfIdx: 1}, &table)
	if route.Outbound || route.Loopback || route.IfIdx != 7 || route.SubIfIdx != 2 || action != virtualRedirectInject {
		t.Fatalf("response rewrite = route %+v action %v", route, action)
	}
	assertTestIPv4TCPPacket(t, responsePacket, rule.VirtualIP, rule.VirtualPort, "192.168.10.20", 51000)
}

func TestRewriteVirtualRedirectPacketAvoidsBridgePortAsNATSource(t *testing.T) {
	rule := virtualRedirectRule{VirtualIP: "203.0.113.10", VirtualPort: 22, LocalPort: 45000}
	table, err := newVirtualRedirectTable([]virtualRedirectRule{rule})
	if err != nil {
		t.Fatal(err)
	}
	packet := testIPv4TCPPacket("192.168.10.20", rule.LocalPort, rule.VirtualIP, rule.VirtualPort)
	if _, action := rewriteVirtualRedirectPacket(packet, virtualRedirectPacketRoute{Outbound: true, IfIdx: 3}, &table); action != virtualRedirectInject {
		t.Fatalf("target action = %v", action)
	}
	if got := binary.BigEndian.Uint16(packet[20:22]); got == rule.LocalPort {
		t.Fatalf("NAT source port reused bridge listener port %d", got)
	}
}

func TestRewriteVirtualRedirectPacketRSTRemovesFlow(t *testing.T) {
	rule := virtualRedirectRule{VirtualIP: "203.0.113.10", VirtualPort: 22, LocalPort: 45000}
	table, err := newVirtualRedirectTable([]virtualRedirectRule{rule})
	if err != nil {
		t.Fatal(err)
	}
	targetPacket := testIPv4TCPPacket("192.168.10.20", 51000, rule.VirtualIP, rule.VirtualPort)
	if _, action := rewriteVirtualRedirectPacket(targetPacket, virtualRedirectPacketRoute{Outbound: true, IfIdx: 3}, &table); action != virtualRedirectInject {
		t.Fatalf("target action = %v", action)
	}
	natPort := binary.BigEndian.Uint16(targetPacket[20:22])
	responsePacket := testIPv4TCPPacket("127.0.0.1", rule.LocalPort, "127.0.0.1", natPort)
	responsePacket[33] = 0x04
	if _, action := rewriteVirtualRedirectPacket(responsePacket, virtualRedirectPacketRoute{Outbound: true, Loopback: true, IfIdx: 1}, &table); action != virtualRedirectInject {
		t.Fatalf("response action = %v", action)
	}
	if len(table.targetFlows) != 0 || len(table.responseFlows) != 0 {
		t.Fatalf("RST left flow mappings: target=%d response=%d", len(table.targetFlows), len(table.responseFlows))
	}
}

func TestRewriteVirtualRedirectPacketSYNReopensClosingFlow(t *testing.T) {
	rule := virtualRedirectRule{VirtualIP: "203.0.113.10", VirtualPort: 22, LocalPort: 45000}
	table, err := newVirtualRedirectTable([]virtualRedirectRule{rule})
	if err != nil {
		t.Fatal(err)
	}
	packet := testIPv4TCPPacket("192.168.10.20", 51000, rule.VirtualIP, rule.VirtualPort)
	if _, action := rewriteVirtualRedirectPacket(packet, virtualRedirectPacketRoute{Outbound: true, IfIdx: 3}, &table); action != virtualRedirectInject {
		t.Fatalf("initial target action = %v", action)
	}
	var flow *virtualRedirectFlow
	for _, current := range table.targetFlows {
		flow = current
	}
	if flow == nil {
		t.Fatal("target flow was not created")
	}
	flow.closing = true

	reopen := testIPv4TCPPacket("192.168.10.20", 51000, rule.VirtualIP, rule.VirtualPort)
	reopen[33] = 0x02
	if _, action := rewriteVirtualRedirectPacket(reopen, virtualRedirectPacketRoute{Outbound: true, IfIdx: 3}, &table); action != virtualRedirectInject {
		t.Fatalf("reopen target action = %v", action)
	}
	if flow.closing {
		t.Fatal("SYN did not clear the closing flow state")
	}
}

func TestVirtualRedirectFlowTableIsBounded(t *testing.T) {
	rule := virtualRedirectRule{VirtualIP: "203.0.113.10", VirtualPort: 22, LocalPort: 45000}
	table, err := newVirtualRedirectTable([]virtualRedirectRule{rule})
	if err != nil {
		t.Fatal(err)
	}
	targetIP := binary.BigEndian.Uint32(net.ParseIP(rule.VirtualIP).To4())
	for i := 0; i <= maxVirtualRedirectFlows; i++ {
		key := virtualRedirectTargetFlowKey{
			SourceIP:   uint32(i + 1),
			SourcePort: uint16(1024 + i%60000),
			TargetIP:   targetIP,
			TargetPort: rule.VirtualPort,
		}
		if _, ok := table.ensureTargetFlow(key, rule, 3, 0); !ok {
			t.Fatalf("flow %d was not allocated", i)
		}
	}
	if got := len(table.targetFlows); got != maxVirtualRedirectFlows {
		t.Fatalf("target flow count = %d, want %d", got, maxVirtualRedirectFlows)
	}
	if got := len(table.responseFlows); got != maxVirtualRedirectFlows {
		t.Fatalf("response flow count = %d, want %d", got, maxVirtualRedirectFlows)
	}
}

func TestRewriteVirtualRedirectPacketPassesExternalAccessBecauseBridgeIsLoopbackOnly(t *testing.T) {
	rule := virtualRedirectRule{VirtualIP: "203.0.113.10", VirtualPort: 22, LocalPort: 45000}
	table, err := newVirtualRedirectTable([]virtualRedirectRule{rule})
	if err != nil {
		t.Fatal(err)
	}
	packet := testIPv4TCPPacket("198.51.100.30", 51000, "192.168.10.20", rule.LocalPort)
	_, action := rewriteVirtualRedirectPacket(packet, virtualRedirectPacketRoute{}, &table)
	if action != virtualRedirectPass {
		t.Fatalf("inbound local redirect action = %v", action)
	}
}

func TestRewriteVirtualRedirectPacketSeparatesSamePortAcrossLocalAddresses(t *testing.T) {
	rule := virtualRedirectRule{VirtualIP: "203.0.113.10", VirtualPort: 22, LocalPort: 45000}
	table, err := newVirtualRedirectTable([]virtualRedirectRule{rule})
	if err != nil {
		t.Fatal(err)
	}
	first := testIPv4TCPPacket("192.168.10.20", 51000, rule.VirtualIP, rule.VirtualPort)
	second := testIPv4TCPPacket("10.0.0.20", 51000, rule.VirtualIP, rule.VirtualPort)
	if _, action := rewriteVirtualRedirectPacket(first, virtualRedirectPacketRoute{Outbound: true, IfIdx: 3}, &table); action != virtualRedirectInject {
		t.Fatalf("first target action = %v", action)
	}
	if _, action := rewriteVirtualRedirectPacket(second, virtualRedirectPacketRoute{Outbound: true, IfIdx: 4}, &table); action != virtualRedirectInject {
		t.Fatalf("second target action = %v", action)
	}
	firstNATPort := binary.BigEndian.Uint16(first[20:22])
	secondNATPort := binary.BigEndian.Uint16(second[20:22])
	if firstNATPort == secondNATPort {
		t.Fatalf("same source port reused NAT port %d across local addresses", firstNATPort)
	}

	firstResponse := testIPv4TCPPacket("127.0.0.1", rule.LocalPort, "127.0.0.1", firstNATPort)
	secondResponse := testIPv4TCPPacket("127.0.0.1", rule.LocalPort, "127.0.0.1", secondNATPort)
	if _, action := rewriteVirtualRedirectPacket(firstResponse, virtualRedirectPacketRoute{Outbound: true, Loopback: true, IfIdx: 1}, &table); action != virtualRedirectInject {
		t.Fatalf("first response action = %v", action)
	}
	if _, action := rewriteVirtualRedirectPacket(secondResponse, virtualRedirectPacketRoute{Outbound: true, Loopback: true, IfIdx: 1}, &table); action != virtualRedirectInject {
		t.Fatalf("second response action = %v", action)
	}
	assertTestIPv4TCPPacket(t, firstResponse, rule.VirtualIP, rule.VirtualPort, "192.168.10.20", 51000)
	assertTestIPv4TCPPacket(t, secondResponse, rule.VirtualIP, rule.VirtualPort, "10.0.0.20", 51000)
}

func TestRewriteVirtualRedirectPacketPassesUnrelatedTunnelPort(t *testing.T) {
	rule := virtualRedirectRule{VirtualIP: "203.0.113.10", VirtualPort: 22, LocalPort: 45000}
	table, err := newVirtualRedirectTable([]virtualRedirectRule{rule})
	if err != nil {
		t.Fatal(err)
	}
	packet := testIPv4TCPPacket("192.168.10.20", 51000, rule.VirtualIP, 3443)
	route, action := rewriteVirtualRedirectPacket(packet, virtualRedirectPacketRoute{Outbound: true, IfIdx: 5}, &table)
	if !route.Outbound || route.IfIdx != 5 || action != virtualRedirectPass {
		t.Fatalf("tunnel packet rewrite = route %+v action %v", route, action)
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
