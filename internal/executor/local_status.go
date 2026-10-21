package executor

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

// Local edition: a read-only look at a server, for the server page.

const nodeStatusScript = `set -Eeuo pipefail
export LC_ALL=C
[ "$(id -u)" = 0 ]
os_id='' os_version=''
if [ -r /etc/os-release ]; then
  os_id=$(. /etc/os-release && printf '%s' "${ID:-}")
  os_version=$(. /etc/os-release && printf '%s' "${VERSION_ID:-}")
fi
printf 'MSBOOST_OS=%s %s\n' "$os_id" "$os_version"
printf 'MSBOOST_BBR=%s\n' "$(sysctl -n net.ipv4.tcp_congestion_control 2>/dev/null || true)"
relays=0
if [ -d /etc/msboost-free ] && [ ! -L /etc/msboost-free ]; then
  relays=$(find /etc/msboost-free -mindepth 1 -maxdepth 1 -type d 2>/dev/null | wc -l)
fi
printf 'MSBOOST_RELAYS=%s\n' "$relays"
if [ ! -f /etc/msboost/install.env ]; then
  printf 'MSBOOST_INSTALLED=0\n'
  exit 0
fi
printf 'MSBOOST_INSTALLED=1\n'
printf 'MSBOOST_SERVICE=%s\n' "$(systemctl is-active msboost.service 2>/dev/null || true)"
printf 'MSBOOST_PROBE=%s\n' "$(systemctl is-active msboost-tcp-probe.service 2>/dev/null || true)"
printf 'MSBOOST_ENGINE=%s\n' "$( (/usr/local/bin/msboost -v 2>/dev/null || true) | head -n 1)"
if [ -f /root/直连.json ] && [ ! -L /root/直连.json ]; then
  printf 'MSBOOST_CONFIG='
  base64 -w0 /root/直连.json
  printf '\n'
fi
`

// NodeStatus is what the server page shows about a server.
type NodeStatus struct {
	OS        string `json:"os"`        // e.g. "Debian 12"
	BBR       string `json:"bbr"`       // "enabled" or "off"
	Installed bool   `json:"installed"` // an MSBOOST node is installed
	Service   string `json:"service"`   // "running" or "stopped"
	Engine    string `json:"engine"`    // engine version, e.g. "v1.19.32"
	Entry     string `json:"entry"`     // the node's address from its configuration
	PublicTCP string `json:"publicTCP"` // "reachable" or "unreachable" from this computer
	// Relays counts the managed relays (relay and front entries) on the server.
	Relays int `json:"relays"`
	// Config is the node's client configuration, when it could be read.
	Config json.RawMessage `json:"-"`
}

var engineVersion = regexp.MustCompile(`v\d+\.\d+\.\d+`)

// NodeStatus reads a server's state without changing anything on it. A
// server without a node is a successful result with Installed false.
func (e *Engine) NodeStatus(ctx context.Context, s SSH) (NodeStatus, Result) {
	var st NodeStatus
	r := Result{}
	e.localDefaults()
	if err := ValidateSSH(s); err != nil {
		return st, failed(r, "validate", err.Error())
	}
	out, err := e.Remote.Run(ctx, s, nodeStatusScript)
	if err != nil {
		return st, failRemote(r, "preflight", out, err)
	}
	if fields := strings.Fields(marker(out, "MSBOOST_OS")); len(fields) > 0 {
		if fields[0] == "debian" {
			fields[0] = "Debian"
		}
		st.OS = strings.Join(fields, " ")
	}
	st.BBR = "off"
	if marker(out, "MSBOOST_BBR") == "bbr" {
		st.BBR = "enabled"
	}
	st.Relays, _ = strconv.Atoi(marker(out, "MSBOOST_RELAYS"))
	r.State, r.Phase = "succeeded", "complete"
	if marker(out, "MSBOOST_INSTALLED") != "1" {
		return st, r
	}
	st.Installed = true
	st.Service = "stopped"
	if marker(out, "MSBOOST_SERVICE") == "active" && marker(out, "MSBOOST_PROBE") == "active" {
		st.Service = "running"
	}
	st.Engine = engineVersion.FindString(marker(out, "MSBOOST_ENGINE"))
	if data, err := base64.StdEncoding.DecodeString(marker(out, "MSBOOST_CONFIG")); err == nil && len(data) > 0 {
		if info, err := ParseClientConfig(data); err == nil {
			st.Config = data
			st.Entry = info.TargetHost + ":" + strconv.Itoa(info.TargetPort)
			st.PublicTCP = e.TCPProbe(ctx, info.TargetHost, info.TargetPort)
		}
	}
	return st, r
}
