package tasks

import (
	"encoding/json"
	"errors"
	"net"
	"regexp"
	"strconv"
	"strings"

	"github.com/mozziexwz/node_local/internal/executor"
)

// The texts friends send each other for a friend relay. They are readable on
// purpose: they hold addresses only, never credentials.

const (
	requestPrefix = "MSBOOST 中转请求 "
	entryPrefix   = "MSBOOST 中转入口 "
	// udpNote ends a text when the node serves UDP, or the relay forwards it.
	// Older versions read only the addresses and pass over it.
	udpNote = " · 支持 UDP"
)

// ShareRequest is what a node's owner sends to the friend who relays.
func ShareRequest(node string, udp bool) string { return requestPrefix + node + note(udp) }

// ShareEntry is what the relay's owner sends back.
func ShareEntry(entry, target string, udp bool) string {
	return entryPrefix + entry + " → " + target + note(udp)
}

func note(udp bool) string {
	if udp {
		return udpNote
	}
	return ""
}

// MentionsUDP says whether pasted text carries the UDP note.
func MentionsUDP(text string) bool { return strings.Contains(text, "UDP") }

// RequestFor is the request text for a configuration.
func RequestFor(config []byte) (string, error) {
	info, err := executor.ParseClientConfig(config)
	if err != nil {
		return "", err
	}
	return ShareRequest(net.JoinHostPort(info.TargetHost, strconv.Itoa(info.TargetPort)), info.UDP), nil
}

var endpoint = regexp.MustCompile(`\[[0-9A-Fa-f:.]+\]:\d{1,5}|(?:\d{1,3}\.){3}\d{1,3}:\d{1,5}`)

// Endpoints finds the host:port pairs in pasted text, in order. Only IP
// addresses count: configurations from MSBOOST name their nodes by IP.
func Endpoints(text string) []string {
	var out []string
	for _, m := range endpoint.FindAllString(text, -1) {
		host, port, err := net.SplitHostPort(m)
		if err != nil || net.ParseIP(host) == nil {
			continue
		}
		out = append(out, net.JoinHostPort(host, port))
	}
	return out
}

// ParseRequest reads the node address a friend sent.
func ParseRequest(text string) (string, error) {
	found := Endpoints(text)
	if len(found) == 0 {
		return "", errors.New("没有找到「IP:端口」，请粘贴朋友发来的整段文字")
	}
	return found[0], nil
}

// ParseEntry reads the text a relay's owner sent back: the entry, and the
// target when the text names one. A bare "IP:port" is an entry without a
// target.
func ParseEntry(text string) (entry, target string, err error) {
	found := Endpoints(text)
	switch {
	case len(found) == 0:
		return "", "", errors.New("没有找到中转入口，请粘贴朋友发回的整段文字")
	case len(found) == 1:
		return found[0], "", nil
	case strings.Contains(text, "→") || strings.Contains(text, "->"):
		return found[0], found[1], nil
	}
	return found[0], "", nil
}

// NodeEntry is a configuration's node address, host:port.
func NodeEntry(config []byte) (string, error) {
	info, err := executor.ParseClientConfig(config)
	if err != nil {
		return "", err
	}
	return net.JoinHostPort(info.TargetHost, strconv.Itoa(info.TargetPort)), nil
}

// FriendCheck is what the node's owner learns from the text sent back.
type FriendCheck struct {
	Entry   string `json:"entry"`   // the relay's entry
	Target  string `json:"target"`  // where the friend says it forwards, "" when not said
	Node    string `json:"node"`    // the node of the chosen configuration
	Matches string `json:"matches"` // yes, no or unknown (no target in the text)
	NodeUDP bool   `json:"nodeUdp"` // the configuration has UDP
	UDP     bool   `json:"udp"`     // and the relay forwards it: the new configuration has UDP too
}

// CheckFriendEntry compares the text a relay's owner sent with the node of
// the configuration it is meant for.
func CheckFriendEntry(config []byte, text string) (FriendCheck, error) {
	info, err := executor.ParseClientConfig(config)
	if err != nil {
		return FriendCheck{}, err
	}
	node := net.JoinHostPort(info.TargetHost, strconv.Itoa(info.TargetPort))
	entry, target, err := ParseEntry(text)
	if err != nil {
		return FriendCheck{}, err
	}
	if entry == node {
		return FriendCheck{}, errors.New("这是你自己节点的地址，不是朋友的中转入口")
	}
	c := FriendCheck{Entry: entry, Target: target, Node: node, Matches: "unknown", NodeUDP: info.UDP, UDP: info.UDP && MentionsUDP(text)}
	if target != "" {
		c.Matches = map[bool]string{true: "yes", false: "no"}[target == node]
	}
	return c, nil
}

// FriendConfig is the configuration with its entry moved to the friend's
// relay; the node and its credentials stay as they were. name becomes the
// profile name, as a relay's remark does on the website.
func FriendConfig(config []byte, text, name string) ([]byte, error) {
	c, err := CheckFriendEntry(config, text)
	if err != nil {
		return nil, err
	}
	if c.Matches == "no" {
		return nil, errors.New("朋友发回的中转目标是 " + c.Target + "，不是这份配置的节点 " + c.Node + "；请确认选对了配置，或让朋友重新发送")
	}
	host, port, _ := net.SplitHostPort(c.Entry)
	n, _ := strconv.Atoi(port)
	out, err := executor.RewriteClientConfig(config, host, n)
	if err == nil && c.UDP {
		out, err = executor.WithUDPBinding(out)
	}
	if err != nil {
		return nil, err
	}
	if name == "" {
		return out, nil
	}
	var doc map[string]any
	if err := json.Unmarshal(out, &doc); err != nil {
		return nil, err
	}
	doc["profiles"].([]any)[0].(map[string]any)["profileName"] = name
	doc["activeProfile"] = name
	return json.MarshalIndent(doc, "", "  ")
}
