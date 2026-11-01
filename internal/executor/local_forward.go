package executor

import (
	"context"
	"net"
	"strconv"
)

// Local edition: a relay to a friend's node from its address alone. The
// friend keeps their configuration and credentials; the relay's owner learns
// only the node's IP and port, which is all the forwarding needs. The relay
// is the same managed relay a "relay" job installs (installRelay), so the
// website and this program list, manage and clean it up alike.

// ForwardRelay installs a managed relay on job.Request.SSH that forwards to
// target:targetPort, at job.Request.RateMbps (0 means the default). udp says
// the friend's node also serves UDP, as the friend's request text tells.
func (e *Engine) ForwardRelay(ctx context.Context, job Job, target string, targetPort int, udp bool) Result {
	r := Result{ID: job.ID, Lease: job.Lease}
	e.localDefaults()
	s := job.Request.SSH
	if err := ValidateSSH(s); err != nil {
		return failed(r, "validate", err.Error())
	}
	rate := job.Request.RateMbps
	if rate < 0 || rate > MaxRelayRateMbps {
		return failed(r, "validate", "")
	}
	if rate == 0 {
		rate = DefaultRelayRateMbps
	}
	if targetPort < 1 || targetPort > 65535 {
		return failRemote(r, "target", nil, diagnosticError("invalid_target"))
	}
	host, err := resolvePublicTarget(ctx, target)
	if err != nil || net.ParseIP(host).Equal(net.ParseIP(s.Host)) {
		return failRemote(r, "target", nil, diagnosticError("invalid_target"))
	}
	if checked, ok := e.preflightDebian(ctx, s, r); !ok {
		return checked
	}
	// UDP as on the website's relays: for a node that serves it, on a port
	// from 1024, once the target's UDP port has stayed silent (see
	// checkUDPTarget).
	asked := RelayUDP(udp, targetPort)
	udp, why := asked, ""
	if udp {
		if verdict := e.checkUDPTarget(ctx, s, host, targetPort); verdict != "silent" {
			udp, why = false, udpTargetReasons[verdict]
		}
	}
	port, udp, bbr, err := e.installRelay(ctx, s, host, targetPort, job.ID+"r", RelayRateBytes(rate), nil, rate, udp)
	r.Health = &Health{BBR: bbr}
	if err != nil {
		return failRemote(r, "relay", nil, err)
	}
	r.Hops = []Hop{{FromHost: s.Host, FromPort: port, ToHost: host, ToPort: targetPort, UDP: udp}}
	r.State, r.Phase = "succeeded", "complete"
	if asked && !udp && why == "" {
		why = "中转服务器没能开 UDP"
	}
	r.Message = "中转已建好；"
	switch {
	case udp:
		r.Message = "中转已建好，TCP 和 UDP 都能用；"
	case why != "":
		r.Message = "中转已建好，只走 TCP：" + why + "。"
	}
	r.Message += "每端口上下行各 " + strconv.Itoa(rate) + " Mbps，游戏连接尚未验证"
	r.Health = &Health{Service: "running", LocalSelfTest: "target_tcp_reachable", PublicTCP: e.TCPProbe(ctx, s.Host, port), Game: "not_tested", BBR: bbr}
	return r
}
