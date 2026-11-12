package executor

import (
	"context"
	"encoding/base64"
	"net"
)

// Local edition: deploy and upgrade with progress.

func (e *Engine) localDefaults() {
	if e.Remote == nil {
		e.Remote = SSHRemote{}
	}
	if e.TCPProbe == nil {
		e.TCPProbe = probeTCP
	}
	// The same check as engine.go's Execute. It runs mihomo, which the engine
	// supports on Linux only, so on this Windows program a configuration with
	// UDP is reported "not_tested"; the page points to the launcher's network
	// check instead.
	if e.UDPProbe == nil {
		e.UDPProbe = e.verifier().ProbeUDP
	}
}

// DeployWithProgress is engine.go's deploy with the installer's stages
// reported through progress. It must remain the same deploy: a test compares
// the scripts both send to the server.
func (e *Engine) DeployWithProgress(ctx context.Context, job Job, progress func(Step)) Result {
	r := Result{ID: job.ID, Lease: job.Lease}
	e.localDefaults()
	if job.Request.Kind != "deploy" {
		return failed(r, "validate", "")
	}
	if err := ValidateRequest(job.Request); err != nil {
		return failed(r, "validate", err.Error())
	}
	if err := verifyAsset(job.Script); err != nil {
		return failed(r, "integrity", err.Error())
	}
	progress(StepConnect)
	if checked, ok := e.preflightDebian(ctx, job.Request.SSH, r); !ok {
		return checked
	}
	progress(StepBBR)
	script := prepareAsset(job.Script) + bbrTuneScript + encodedAssignment("MSBOOST_SERVER_IP", job.Request.SSH.Host) + "export MSBOOST_SERVER_IP\nmsboost_phase=install\n"
	script += encodedAssignment("node_user", "msboost-"+randomHex(8)) + encodedAssignment("node_pass", randomHex(24))
	script += "MSBOOST_FORCE_FRESH=1 MSBOOST_UDP=1 bash \"$work/installer.sh\" \"$node_user\" \"$node_pass\" " + progressPipe + "\n"
	script += installedTail + fail2banStep(job.Request.Fail2ban)
	out, err := e.runWithProgress(ctx, job.Request.SSH, script, progress)
	r.Health = &Health{BBR: marker(out, "MSBOOST_BBR")}
	if err != nil {
		return failRemote(r, "install", out, err)
	}
	progress(StepConfig)
	data, err := base64.StdEncoding.DecodeString(marker(out, "MSBOOST_CONFIG"))
	if err != nil || len(data) == 0 {
		return failed(r, "config", "未取得经过验证的客户端配置")
	}
	info, err := ParseClientConfig(data)
	if err != nil {
		return failed(r, "config", "安装结果的客户端配置格式不符合要求")
	}
	if info.TargetHost != job.Request.SSH.Host {
		return failed(r, "config", "安装结果入口与已确认的服务器不一致")
	}
	r.State = "succeeded"
	r.Phase = "complete"
	r.Message = "节点服务及全程 TCP 测速协议自测通过；公网 TCP 和 UDP 单独检查，游戏连通性尚未验证"
	r.Config = data
	r.Health = &Health{Service: "running", LocalSelfTest: "passed", PublicTCP: e.TCPProbe(ctx, info.TargetHost, info.TargetPort), PublicUDP: e.UDPProbe(ctx, data), Game: "not_tested", BBR: marker(out, "MSBOOST_BBR")}
	if job.Request.Fail2ban != nil {
		r.Health.Fail2ban = fail2banStatus(out)
	}
	return r
}

// UpgradeWithProgress reruns the bundled installer on a deployed node without
// MSBOOST_FORCE_FRESH, which keeps the recorded user, password, port and
// low-entropy level (see the installer's usage notes). The node's current
// configuration is read first: its entry IP becomes MSBOOST_SERVER_IP, and the
// configuration afterwards must keep the same entry and credentials, so the
// user's existing configuration file keeps working.
func (e *Engine) UpgradeWithProgress(ctx context.Context, job Job, progress func(Step)) Result {
	r := Result{ID: job.ID, Lease: job.Lease}
	e.localDefaults()
	s := job.Request.SSH
	if err := ValidateSSH(s); err != nil {
		return failed(r, "validate", err.Error())
	}
	if err := ValidateFail2ban(job.Request.Fail2ban); err != nil {
		return failed(r, "validate", err.Error())
	}
	if err := verifyAsset(job.Script); err != nil {
		return failed(r, "integrity", err.Error())
	}
	progress(StepConnect)
	before := e.FetchConfig(ctx, s)
	if before.State != "succeeded" {
		before.ID, before.Lease, before.Health = r.ID, r.Lease, nil
		return before
	}
	old, err := ParseClientConfig(before.Config)
	if err != nil {
		return failed(r, "config", "")
	}
	progress(StepPreflight)
	if checked, ok := e.preflightDebian(ctx, s, r); !ok {
		return checked
	}
	progress(StepBBR)
	script := prepareAsset(job.Script) + bbrTuneScript
	if net.ParseIP(old.TargetHost) != nil {
		script += encodedAssignment("MSBOOST_SERVER_IP", old.TargetHost) + "export MSBOOST_SERVER_IP\n"
	}
	script += "msboost_phase=install\n[ -f /etc/msboost/install.env ]\n"
	// An upgrade adds UDP to a TCP-only node when its port is free for it.
	script += "MSBOOST_UDP=1 bash \"$work/installer.sh\" " + progressPipe + "\n"
	script += installedTail + fail2banStep(job.Request.Fail2ban)
	out, err := e.runWithProgress(ctx, s, script, progress)
	r.Health = &Health{BBR: marker(out, "MSBOOST_BBR")}
	if err != nil {
		return failRemote(r, "install", out, err)
	}
	progress(StepConfig)
	data, err := base64.StdEncoding.DecodeString(marker(out, "MSBOOST_CONFIG"))
	if err != nil || len(data) == 0 {
		return failed(r, "config", "")
	}
	info, err := ParseClientConfig(data)
	if err != nil {
		return failed(r, "config", "")
	}
	if info.TargetHost != old.TargetHost || info.TargetPort != old.TargetPort || info.Username != old.Username || info.Password != old.Password {
		return failRemote(r, "config", nil, diagnosticError("config_changed"))
	}
	r.State = "succeeded"
	r.Phase = "complete"
	// The existing file keeps working either way. The new one replaces it
	// where the launcher reads it; with UDP it needs launcher v1.2.0, as older
	// launchers mix TCP and UDP.
	r.Message = "节点已升级，入口和认证没变，原配置继续可用；公网 TCP 和 UDP 单独检查，游戏连通性尚未验证"
	if info.UDP && !old.UDP {
		r.Message = "节点已升级并开启了 UDP，入口和认证没变；新配置同时支持 TCP 和 UDP，启动器要 v1.2.0 或更新版本。公网 TCP 和 UDP 单独检查，游戏连通性尚未验证"
	}
	r.Config = data
	r.Health = &Health{Service: "running", LocalSelfTest: "passed", PublicTCP: e.TCPProbe(ctx, info.TargetHost, info.TargetPort), PublicUDP: e.UDPProbe(ctx, data), Game: "not_tested", BBR: marker(out, "MSBOOST_BBR")}
	if job.Request.Fail2ban != nil {
		r.Health.Fail2ban = fail2banStatus(out)
	}
	return r
}
