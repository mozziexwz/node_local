package tasks

import (
	"strings"
	"testing"

	"github.com/mozziexwz/node_local/internal/executor"
)

// nodeConfigUDP is nodeConfig for a node that also serves UDP.
var nodeConfigUDP = strings.Replace(nodeConfig, `{"port":30000,"protocol":"TCP"}`, `{"port":30000,"protocol":"TCP"},{"port":30000,"protocol":"UDP"}`, 1)

func TestShareTexts(t *testing.T) {
	if got := ShareRequest("203.0.113.10:34917", false); got != "MSBOOST 中转请求 203.0.113.10:34917" {
		t.Fatal(got)
	}
	if got := ShareRequest("203.0.113.10:34917", true); got != "MSBOOST 中转请求 203.0.113.10:34917 · 支持 UDP" || !MentionsUDP(got) {
		t.Fatal(got)
	}
	for text, want := range map[string]string{
		"MSBOOST 中转请求 203.0.113.10:34917":          "203.0.113.10:34917",
		"MSBOOST 中转请求 203.0.113.10:34917 · 支持 UDP": "203.0.113.10:34917",
		"  203.0.113.10:34917 ":                    "203.0.113.10:34917",
		"节点在 [2001:db8::1]:443，帮我转一下":              "[2001:db8::1]:443",
	} {
		if got, err := ParseRequest(text); err != nil || got != want {
			t.Errorf("ParseRequest(%q) = %q, %v", text, got, err)
		}
	}
	if _, err := ParseRequest("没有地址"); err == nil {
		t.Error("accepted text without an address")
	}

	// Older versions read the same addresses from a text with the UDP note.
	for _, udp := range []bool{false, true} {
		share := ShareEntry("198.51.100.88:41203", "203.0.113.10:34917", udp)
		entry, target, err := ParseEntry(share)
		if err != nil || entry != "198.51.100.88:41203" || target != "203.0.113.10:34917" || MentionsUDP(share) != udp {
			t.Fatalf("ParseEntry(%q) = %q %q %v", share, entry, target, err)
		}
	}
	if entry, target, err := ParseEntry("198.51.100.88:41203"); err != nil || entry != "198.51.100.88:41203" || target != "" {
		t.Fatalf("bare entry: %q %q %v", entry, target, err)
	}
	if _, _, err := ParseEntry("hello"); err == nil {
		t.Fatal("accepted text without an entry")
	}

	for config, want := range map[string]string{nodeConfig: "MSBOOST 中转请求 8.8.8.8:30000", nodeConfigUDP: "MSBOOST 中转请求 8.8.8.8:30000 · 支持 UDP"} {
		if got, err := RequestFor([]byte(config)); err != nil || got != want {
			t.Errorf("RequestFor = %q, %v; want %q", got, err, want)
		}
	}
}

func TestFriendConfig(t *testing.T) {
	check, err := CheckFriendEntry([]byte(nodeConfig), ShareEntry("1.1.1.1:41203", "8.8.8.8:30000", false))
	if err != nil || check != (FriendCheck{Entry: "1.1.1.1:41203", Target: "8.8.8.8:30000", Node: "8.8.8.8:30000", Matches: "yes"}) {
		t.Fatalf("check %+v %v", check, err)
	}
	if c, _ := CheckFriendEntry([]byte(nodeConfig), "1.1.1.1:41203"); c.Matches != "unknown" {
		t.Fatalf("bare entry %+v", c)
	}
	if _, err := CheckFriendEntry([]byte(nodeConfig), "8.8.8.8:30000"); err == nil {
		t.Fatal("the node's own address was taken for an entry")
	}

	out, err := FriendConfig([]byte(nodeConfig), ShareEntry("1.1.1.1:41203", "8.8.8.8:30000", false), "东京节点-经小明中转")
	if err != nil {
		t.Fatal(err)
	}
	info, err := executor.ParseClientConfig(out)
	if err != nil || info.TargetHost != "1.1.1.1" || info.TargetPort != 41203 || info.Username != "msboost-a1" || info.Password != "node-secret" || info.UDP {
		t.Fatalf("rewritten %s %v", out, err)
	}
	if !strings.Contains(string(out), `"activeProfile": "东京节点-经小明中转"`) {
		t.Fatalf("profile name not set: %s", out)
	}
	if _, err := FriendConfig([]byte(nodeConfig), ShareEntry("1.1.1.1:41203", "9.9.9.9:1", false), ""); err == nil || !strings.Contains(err.Error(), "不是这份配置的节点") {
		t.Fatalf("mismatched target: %v", err)
	}
}

func TestFriendConfigWithUDP(t *testing.T) {
	for name, tc := range map[string]struct {
		config   string
		relayUDP bool
		nodeUDP  bool
		udp      bool
	}{
		"both":           {nodeConfigUDP, true, true, true},
		"relay TCP only": {nodeConfigUDP, false, true, false},
		"node TCP only":  {nodeConfig, true, false, false},
	} {
		text := ShareEntry("1.1.1.1:41203", "8.8.8.8:30000", tc.relayUDP)
		check, err := CheckFriendEntry([]byte(tc.config), text)
		if err != nil || check.Matches != "yes" || check.NodeUDP != tc.nodeUDP || check.UDP != tc.udp {
			t.Errorf("%s: check %+v %v", name, check, err)
			continue
		}
		out, err := FriendConfig([]byte(tc.config), text, "")
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		// The UDP binding, when there is one, is on the relay's entry.
		info, err := executor.ParseClientConfig(out)
		if err != nil || info.TargetHost != "1.1.1.1" || info.TargetPort != 41203 || info.UDP != tc.udp {
			t.Errorf("%s: rewritten %s %v", name, out, err)
		}
	}
}
