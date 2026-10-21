package tasks

import "github.com/mozziexwz/node_local/internal/store"

// The engine's diagnostics were written for the website, where a remote
// executor connects and an administrator helps. These say the same for a
// program running on the user's own computer.
var localTexts = map[string][2]string{
	"ssh_timeout":            {"SSH 连接超时", "检查 VPS 是否开机、SSH 端口和服务商安全组是否放行。国内直连不稳定时，可以在「设置与关于」里打开 SSH 代理。"},
	"ssh_connect":            {"无法连接 SSH 服务", "检查 IP、端口、VPS 网络和服务商安全组。国内直连不稳定时，可以在「设置与关于」里打开 SSH 代理。"},
	"ssh_host_changed":       {"服务器指纹变了，已停止连接", "先在 VPS 服务商控制台核实是否重装过系统或换了机器；确认无误后，在服务器详情里重新读取并确认新指纹。"},
	"task_timeout":           {"操作超时或连接中断", "服务器上的操作可能还在继续。稍后用「刷新状态」或「取回配置」确认，不要马上重复部署。"},
	"execution_failed":       {"服务器上的步骤失败了，没有足够的信息判断原因", "可以稍后重试；多次失败的话，请在 VPS 上检查系统、磁盘和网络。"},
	"integrity_failed":       {"安装资源校验失败", "请从官方渠道重新下载 MSBOOST 本地版。"},
	"executor_configuration": {"程序缺少固定版本的下载信息", "请从官方渠道重新下载 MSBOOST 本地版。"},
	"executor_offline":       {"连接中断", "服务器上的操作可能还在继续。稍后用「刷新状态」确认。"},
}

const proxyHint = "已开启 SSH 代理：请确认代理软件正在运行，并核对「设置与关于」里的代理地址和端口。"

// localize replaces website wording in a failed outcome.
func localize(o *Outcome, settings store.Settings) {
	if t, ok := localTexts[o.ErrorCode]; ok {
		o.Message, o.NextStep = t[0], t[1]
	}
	if settings.Proxy.Enabled {
		switch o.ErrorCode {
		case "ssh_timeout", "ssh_connect", "ssh_refused", "ssh_handshake":
			o.NextStep = proxyHint
		}
	}
}
