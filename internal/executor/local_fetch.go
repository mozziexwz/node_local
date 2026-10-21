package executor

import (
	"context"
	"encoding/base64"
)

// Local edition: read a deployed node's client configuration without changing
// the server. A deploy can finish on the server after the SSH connection from
// this computer dropped; the configuration is then still on the node.
const fetchConfigScript = `set -Eeuo pipefail
export LC_ALL=C
[ "$(id -u)" = 0 ]
if [ ! -f /etc/msboost/install.env ] || [ ! -f /root/直连.json ] || [ -L /root/直连.json ]; then
  printf 'MSBOOST_ERROR_CODE=not_installed\nMSBOOST_ERROR_PHASE=config\n'
  exit 1
fi
printf 'MSBOOST_SERVICE=%s\n' "$(systemctl is-active msboost.service 2>/dev/null || true)"
printf 'MSBOOST_PROBE=%s\n' "$(systemctl is-active msboost-tcp-probe.service 2>/dev/null || true)"
printf 'MSBOOST_CONFIG='
base64 -w0 /root/直连.json
printf '\n'
`

// FetchConfig returns the node's current client configuration, checked the
// same way a deploy checks it, plus whether both services are running.
func (e *Engine) FetchConfig(ctx context.Context, s SSH) Result {
	r := Result{}
	if e.Remote == nil {
		e.Remote = SSHRemote{}
	}
	if e.TCPProbe == nil {
		e.TCPProbe = probeTCP
	}
	if err := ValidateSSH(s); err != nil {
		return failed(r, "validate", err.Error())
	}
	out, err := e.Remote.Run(ctx, s, fetchConfigScript)
	if err != nil {
		return failRemote(r, "config", out, err)
	}
	data, err := base64.StdEncoding.DecodeString(marker(out, "MSBOOST_CONFIG"))
	if err != nil || len(data) == 0 {
		return failed(r, "config", "未取得节点配置")
	}
	info, err := ParseClientConfig(data)
	if err != nil {
		return failed(r, "config", "节点配置格式不符合要求")
	}
	if info.TargetHost != s.Host {
		return failRemote(r, "config", nil, diagnosticError("config_host_mismatch"))
	}
	service := "stopped"
	if marker(out, "MSBOOST_SERVICE") == "active" && marker(out, "MSBOOST_PROBE") == "active" {
		service = "running"
	}
	r.State, r.Phase, r.Config = "succeeded", "complete", data
	r.Message = "已从节点取回配置；公网 TCP 单独检查，游戏连通性尚未验证"
	r.Health = &Health{Service: service, LocalSelfTest: "not_tested", PublicTCP: e.TCPProbe(ctx, info.TargetHost, info.TargetPort), Game: "not_tested"}
	return r
}
