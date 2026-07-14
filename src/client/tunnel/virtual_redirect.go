package tunnel

import (
	"encoding/binary"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"
)

type virtualRedirectSession interface {
	Close() error
	Done() <-chan struct{}
	Err() error
}

type virtualRedirectRule struct {
	VirtualIP   string `json:"virtual_ip"`
	VirtualPort uint16 `json:"virtual_port"`
	LocalPort   uint16 `json:"local_port"`
}

type virtualRedirectPacketAction uint8

const (
	// Each endpoint expands into several WinDivert filter tests; keep headroom below its 256-object limit.
	maxVirtualRedirectEndpoints = 48
	maxVirtualRedirectFlows     = 8192
	virtualRedirectEvictSample  = 32
	virtualRedirectFlowIdleTTL  = 24 * time.Hour
	virtualRedirectClosingTTL   = 10 * time.Minute
	virtualRedirectSweepPeriod  = time.Minute
	virtualRedirectLoopbackIPv4 = uint32(0x7f000001)

	virtualRedirectPass virtualRedirectPacketAction = iota
	virtualRedirectInject
	virtualRedirectDrop
)

type virtualRedirectEndpointKey struct {
	IP   uint32
	Port uint16
}

type virtualRedirectTable struct {
	targets         map[virtualRedirectEndpointKey]virtualRedirectRule
	responses       map[uint16]virtualRedirectRule
	targetFlows     map[virtualRedirectTargetFlowKey]*virtualRedirectFlow
	responseFlows   map[virtualRedirectResponseFlowKey]*virtualRedirectFlow
	nextNATPort     uint16
	lastFlowCleanup time.Time
}

type virtualRedirectTargetFlowKey struct {
	SourceIP   uint32
	SourcePort uint16
	TargetIP   uint32
	TargetPort uint16
}

type virtualRedirectResponseFlowKey struct {
	LocalPort uint16
	NATPort   uint16
}

type virtualRedirectFlow struct {
	targetKey   virtualRedirectTargetFlowKey
	responseKey virtualRedirectResponseFlowKey
	rule        virtualRedirectRule
	lastSeen    time.Time
	closing     bool
	ifIdx       uint32
	subIfIdx    uint32
}

type virtualRedirectPacketRoute struct {
	Outbound bool
	Loopback bool
	IfIdx    uint32
	SubIfIdx uint32
}

var startVirtualRedirectSessionFn = startVirtualRedirectSession

func newVirtualRedirectRule(virtualAddr string, localPort int) (virtualRedirectRule, error) {
	host, portText, err := net.SplitHostPort(strings.TrimSpace(virtualAddr))
	if err != nil {
		return virtualRedirectRule{}, fmt.Errorf("parse virtual redirect endpoint: %w", err)
	}
	ip, err := normalizeVirtualIPv4(host)
	if err != nil {
		return virtualRedirectRule{}, err
	}
	virtualPort, err := strconv.Atoi(portText)
	if err != nil || virtualPort < 1 || virtualPort > 65535 {
		return virtualRedirectRule{}, fmt.Errorf("virtual redirect port must be between 1 and 65535")
	}
	if localPort < 1 || localPort > 65535 {
		return virtualRedirectRule{}, fmt.Errorf("local virtual redirect port must be between 1 and 65535")
	}
	return virtualRedirectRule{
		VirtualIP:   ip.String(),
		VirtualPort: uint16(virtualPort),
		LocalPort:   uint16(localPort),
	}, nil
}

func normalizeVirtualRedirectRules(values []virtualRedirectRule) ([]virtualRedirectRule, error) {
	if len(values) > maxVirtualRedirectEndpoints {
		return nil, fmt.Errorf("virtual forwarding supports at most %d endpoints", maxVirtualRedirectEndpoints)
	}
	targets := make(map[string]struct{}, len(values))
	localPorts := make(map[uint16]struct{}, len(values))
	rules := make([]virtualRedirectRule, 0, len(values))
	for _, value := range values {
		ip, err := normalizeVirtualIPv4(value.VirtualIP)
		if err != nil {
			return nil, err
		}
		if value.VirtualPort == 0 || value.LocalPort == 0 {
			return nil, fmt.Errorf("virtual redirect ports must be between 1 and 65535")
		}
		value.VirtualIP = ip.String()
		key := net.JoinHostPort(value.VirtualIP, strconv.Itoa(int(value.VirtualPort)))
		if _, exists := targets[key]; exists {
			return nil, fmt.Errorf("duplicate virtual redirect endpoint %s", key)
		}
		if _, exists := localPorts[value.LocalPort]; exists {
			return nil, fmt.Errorf("duplicate local virtual redirect port %d", value.LocalPort)
		}
		targets[key] = struct{}{}
		localPorts[value.LocalPort] = struct{}{}
		rules = append(rules, value)
	}
	sort.Slice(rules, func(i, j int) bool {
		if rules[i].VirtualIP != rules[j].VirtualIP {
			return rules[i].VirtualIP < rules[j].VirtualIP
		}
		return rules[i].VirtualPort < rules[j].VirtualPort
	})
	return rules, nil
}

func newVirtualRedirectTable(rules []virtualRedirectRule) (virtualRedirectTable, error) {
	rules, err := normalizeVirtualRedirectRules(rules)
	if err != nil {
		return virtualRedirectTable{}, err
	}
	table := virtualRedirectTable{
		targets:         make(map[virtualRedirectEndpointKey]virtualRedirectRule, len(rules)),
		responses:       make(map[uint16]virtualRedirectRule, len(rules)),
		targetFlows:     make(map[virtualRedirectTargetFlowKey]*virtualRedirectFlow),
		responseFlows:   make(map[virtualRedirectResponseFlowKey]*virtualRedirectFlow),
		nextNATPort:     49152,
		lastFlowCleanup: time.Now(),
	}
	for _, rule := range rules {
		ip := binary.BigEndian.Uint32(net.ParseIP(rule.VirtualIP).To4())
		table.targets[virtualRedirectEndpointKey{IP: ip, Port: rule.VirtualPort}] = rule
		table.responses[rule.LocalPort] = rule
	}
	return table, nil
}

func buildVirtualRedirectFilter(rules []virtualRedirectRule) (string, error) {
	rules, err := normalizeVirtualRedirectRules(rules)
	if err != nil {
		return "", err
	}
	if len(rules) == 0 {
		return "", fmt.Errorf("at least one virtual redirect rule is required")
	}
	targets := make([]string, 0, len(rules))
	responses := make([]string, 0, len(rules))
	for _, rule := range rules {
		targets = append(targets, fmt.Sprintf("(ip.DstAddr == %s and tcp.DstPort == %d)", rule.VirtualIP, rule.VirtualPort))
		responses = append(responses, fmt.Sprintf("(loopback and ip.SrcAddr == 127.0.0.1 and ip.DstAddr == 127.0.0.1 and tcp.SrcPort == %d)", rule.LocalPort))
	}
	return "ip and tcp and outbound and (" + strings.Join(append(targets, responses...), " or ") + ")", nil
}

func rewriteVirtualRedirectPacket(packet []byte, route virtualRedirectPacketRoute, table *virtualRedirectTable) (virtualRedirectPacketRoute, virtualRedirectPacketAction) {
	if table == nil {
		return route, virtualRedirectPass
	}
	ihl, srcIP, dstIP, srcPort, dstPort, ok := parseVirtualRedirectIPv4TCP(packet)
	if !ok {
		return route, virtualRedirectPass
	}
	tcpFlags := packet[ihl+13]
	srcIPValue := binary.BigEndian.Uint32(srcIP)
	srcPortValue := binary.BigEndian.Uint16(srcPort)
	dstPortValue := binary.BigEndian.Uint16(dstPort)
	dstIPValue := binary.BigEndian.Uint32(dstIP)

	if !route.Outbound {
		return route, virtualRedirectPass
	}

	if rule, found := table.targets[virtualRedirectEndpointKey{IP: dstIPValue, Port: dstPortValue}]; found {
		flow, ok := table.ensureTargetFlow(virtualRedirectTargetFlowKey{
			SourceIP:   srcIPValue,
			SourcePort: srcPortValue,
			TargetIP:   dstIPValue,
			TargetPort: dstPortValue,
		}, rule, route.IfIdx, route.SubIfIdx)
		if !ok {
			return route, virtualRedirectDrop
		}
		binary.BigEndian.PutUint32(srcIP, virtualRedirectLoopbackIPv4)
		binary.BigEndian.PutUint16(srcPort, flow.responseKey.NATPort)
		binary.BigEndian.PutUint32(dstIP, virtualRedirectLoopbackIPv4)
		binary.BigEndian.PutUint16(dstPort, rule.LocalPort)
		table.updateFlowState(flow, tcpFlags)
		return virtualRedirectPacketRoute{Outbound: true, Loopback: true, IfIdx: 1}, virtualRedirectInject
	}
	if rule, found := table.responses[srcPortValue]; found && route.Loopback && srcIPValue == virtualRedirectLoopbackIPv4 && dstIPValue == virtualRedirectLoopbackIPv4 {
		flow := table.responseFlows[virtualRedirectResponseFlowKey{
			LocalPort: srcPortValue,
			NATPort:   dstPortValue,
		}]
		if flow == nil || flow.rule != rule {
			return route, virtualRedirectPass
		}
		binary.BigEndian.PutUint32(srcIP, flow.targetKey.TargetIP)
		binary.BigEndian.PutUint16(srcPort, rule.VirtualPort)
		binary.BigEndian.PutUint32(dstIP, flow.targetKey.SourceIP)
		binary.BigEndian.PutUint16(dstPort, flow.targetKey.SourcePort)
		table.updateFlowState(flow, tcpFlags)
		return virtualRedirectPacketRoute{IfIdx: flow.ifIdx, SubIfIdx: flow.subIfIdx}, virtualRedirectInject
	}
	return route, virtualRedirectPass
}

func (t *virtualRedirectTable) ensureTargetFlow(key virtualRedirectTargetFlowKey, rule virtualRedirectRule, ifIdx, subIfIdx uint32) (*virtualRedirectFlow, bool) {
	now := time.Now()
	t.cleanupFlows(now)
	if flow := t.targetFlows[key]; flow != nil {
		flow.lastSeen = now
		flow.ifIdx = ifIdx
		flow.subIfIdx = subIfIdx
		return flow, true
	}
	if len(t.targetFlows) >= maxVirtualRedirectFlows {
		t.evictStaleFlow()
	}
	natPort, ok := t.allocateNATPort(key.SourcePort, rule.LocalPort)
	if !ok {
		return nil, false
	}
	responseKey := virtualRedirectResponseFlowKey{LocalPort: rule.LocalPort, NATPort: natPort}
	flow := &virtualRedirectFlow{
		targetKey:   key,
		responseKey: responseKey,
		rule:        rule,
		lastSeen:    now,
		ifIdx:       ifIdx,
		subIfIdx:    subIfIdx,
	}
	t.targetFlows[key] = flow
	t.responseFlows[responseKey] = flow
	return flow, true
}

func (t *virtualRedirectTable) evictStaleFlow() {
	var candidate *virtualRedirectFlow
	checked := 0
	for _, flow := range t.targetFlows {
		if candidate == nil || flow.lastSeen.Before(candidate.lastSeen) {
			candidate = flow
		}
		checked++
		if checked >= virtualRedirectEvictSample {
			break
		}
	}
	t.removeFlow(candidate)
}

func (t *virtualRedirectTable) allocateNATPort(preferred uint16, localPort uint16) (uint16, bool) {
	if preferred != 0 && preferred != localPort {
		key := virtualRedirectResponseFlowKey{LocalPort: localPort, NATPort: preferred}
		if t.responseFlows[key] == nil {
			return preferred, true
		}
	}
	for attempts := 0; attempts < 65535; attempts++ {
		port := t.nextNATPort
		t.nextNATPort++
		if t.nextNATPort == 0 {
			t.nextNATPort = 1024
		}
		if port == 0 || port == localPort {
			continue
		}
		key := virtualRedirectResponseFlowKey{LocalPort: localPort, NATPort: port}
		if t.responseFlows[key] == nil {
			return port, true
		}
	}
	return 0, false
}

func (t *virtualRedirectTable) updateFlowState(flow *virtualRedirectFlow, tcpFlags byte) {
	flow.lastSeen = time.Now()
	if tcpFlags&0x02 != 0 {
		flow.closing = false
	}
	if tcpFlags&0x04 != 0 {
		t.removeFlow(flow)
		return
	}
	if tcpFlags&0x01 != 0 {
		flow.closing = true
	}
}

func (t *virtualRedirectTable) cleanupFlows(now time.Time) {
	if now.Sub(t.lastFlowCleanup) < virtualRedirectSweepPeriod {
		return
	}
	t.lastFlowCleanup = now
	for _, flow := range t.targetFlows {
		ttl := virtualRedirectFlowIdleTTL
		if flow.closing {
			ttl = virtualRedirectClosingTTL
		}
		if now.Sub(flow.lastSeen) >= ttl {
			t.removeFlow(flow)
		}
	}
}

func (t *virtualRedirectTable) removeFlow(flow *virtualRedirectFlow) {
	if flow == nil {
		return
	}
	delete(t.targetFlows, flow.targetKey)
	delete(t.responseFlows, flow.responseKey)
}

func parseVirtualRedirectIPv4TCP(packet []byte) (int, []byte, []byte, []byte, []byte, bool) {
	if len(packet) < 40 || packet[0]>>4 != 4 || packet[9] != 6 {
		return 0, nil, nil, nil, nil, false
	}
	ihl := int(packet[0]&0x0f) * 4
	if ihl < 20 || len(packet) < ihl+20 {
		return 0, nil, nil, nil, nil, false
	}
	return ihl, packet[12:16], packet[16:20], packet[ihl : ihl+2], packet[ihl+2 : ihl+4], true
}
