package tunnel

import (
	"encoding/binary"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
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

	virtualRedirectPass virtualRedirectPacketAction = iota
	virtualRedirectInject
	virtualRedirectDrop
)

type virtualRedirectEndpointKey struct {
	IP   uint32
	Port uint16
}

type virtualRedirectTable struct {
	targets    map[virtualRedirectEndpointKey]virtualRedirectRule
	responses  map[virtualRedirectEndpointKey]virtualRedirectRule
	localPorts map[uint16]struct{}
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
		targets:    make(map[virtualRedirectEndpointKey]virtualRedirectRule, len(rules)),
		responses:  make(map[virtualRedirectEndpointKey]virtualRedirectRule, len(rules)),
		localPorts: make(map[uint16]struct{}, len(rules)),
	}
	for _, rule := range rules {
		ip := binary.BigEndian.Uint32(net.ParseIP(rule.VirtualIP).To4())
		table.targets[virtualRedirectEndpointKey{IP: ip, Port: rule.VirtualPort}] = rule
		table.responses[virtualRedirectEndpointKey{IP: ip, Port: rule.LocalPort}] = rule
		table.localPorts[rule.LocalPort] = struct{}{}
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
	inboundPorts := make([]string, 0, len(rules))
	for _, rule := range rules {
		targets = append(targets, fmt.Sprintf("(ip.DstAddr == %s and tcp.DstPort == %d)", rule.VirtualIP, rule.VirtualPort))
		responses = append(responses, fmt.Sprintf("(ip.DstAddr == %s and tcp.SrcPort == %d)", rule.VirtualIP, rule.LocalPort))
		inboundPorts = append(inboundPorts, fmt.Sprintf("tcp.DstPort == %d", rule.LocalPort))
	}
	return "ip and tcp and ((outbound and (" + strings.Join(append(targets, responses...), " or ") + ")) or (inbound and not impostor and (" + strings.Join(inboundPorts, " or ") + ")))", nil
}

func rewriteVirtualRedirectPacket(packet []byte, outbound bool, table virtualRedirectTable) (bool, virtualRedirectPacketAction) {
	ihl, srcIP, dstIP, srcPort, dstPort, ok := parseVirtualRedirectIPv4TCP(packet)
	if !ok {
		return outbound, virtualRedirectPass
	}
	_ = ihl
	srcPortValue := binary.BigEndian.Uint16(srcPort)
	dstPortValue := binary.BigEndian.Uint16(dstPort)
	dstIPValue := binary.BigEndian.Uint32(dstIP)

	if !outbound {
		if _, protected := table.localPorts[dstPortValue]; protected {
			return false, virtualRedirectDrop
		}
		return false, virtualRedirectPass
	}

	if rule, found := table.targets[virtualRedirectEndpointKey{IP: dstIPValue, Port: dstPortValue}]; found {
		binary.BigEndian.PutUint16(dstPort, rule.LocalPort)
		swapIPv4Addresses(srcIP, dstIP)
		return false, virtualRedirectInject
	}
	if rule, found := table.responses[virtualRedirectEndpointKey{IP: dstIPValue, Port: srcPortValue}]; found {
		binary.BigEndian.PutUint16(srcPort, rule.VirtualPort)
		swapIPv4Addresses(srcIP, dstIP)
		return false, virtualRedirectInject
	}
	return true, virtualRedirectPass
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

func swapIPv4Addresses(left, right []byte) {
	var value [4]byte
	copy(value[:], left)
	copy(left, right)
	copy(right, value[:])
}
