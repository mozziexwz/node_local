package executor

// Local edition: diagnostics for its own tasks, added to the closed vocabulary
// without editing the synced diagnostics.go.
func init() {
	for _, d := range []Diagnostic{
		{"not_installed", "config", "这台服务器上没有找到 MSBOOST 节点", "先部署 MSBOOST；如果确实部署过，检查是否连错了服务器。"},
		{"config_host_mismatch", "config", "节点配置里的入口地址不是这台服务器", "这台节点可能用其他 IP 部署过（多 IP 或 NAT 机器）；请核对服务器地址。"},
		{"config_changed", "config", "升级后节点配置发生了变化", "用「取回配置」拿到新配置，再在启动器里换掉旧配置。"},
		{"invalid_target", "target", "朋友节点的地址无效", "请确认朋友发来的是公网 IP 和端口，例如 203.0.113.10:34917；也不能转发到这台中转服务器自己。"},
	} {
		diagnostics[d.Code] = d
	}
}
