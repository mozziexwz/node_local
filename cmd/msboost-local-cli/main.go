// Command msboost-local-cli runs the local edition's engine against a real VPS
// from the command line, for testing before the desktop interface exists. It
// is not part of the release.
//
//	msboost-local-cli fingerprint -host <IP> [-port 22]
//	msboost-local-cli deploy  -host <IP> [-port 22] [-key <私钥文件>] [-fingerprint SHA256:...] [-out <文件>]
//	msboost-local-cli fetch   -host <IP> [-port 22] [-key <私钥文件>] [-fingerprint SHA256:...] [-out <文件>]
//	msboost-local-cli cleanup -host <IP> [-port 22] [-key <私钥文件>] [-fingerprint SHA256:...] [-scope msboost|relay]
//
// The SSH password or key passphrase is read from the console without echo and
// is never taken as an argument. Without -fingerprint the server's host key is
// shown first and must be confirmed.
package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/term"

	"github.com/mozziexwz/node_local/installers"
	"github.com/mozziexwz/node_local/internal/executor"
)

var version = "dev"

var stdin = bufio.NewReader(os.Stdin)

const usage = `MSBOOST 本地版测试工具 %s

用法：
  msboost-local-cli fingerprint -host <IP> [-port 22]
  msboost-local-cli deploy  -host <IP> [-port 22] [-key <私钥文件>] [-fingerprint SHA256:...] [-out <文件>]
  msboost-local-cli fetch   -host <IP> [-port 22] [-key <私钥文件>] [-fingerprint SHA256:...] [-out <文件>]
  msboost-local-cli cleanup -host <IP> [-port 22] [-key <私钥文件>] [-fingerprint SHA256:...] [-scope msboost|relay]

  fingerprint  只读取服务器的 SSH 指纹
  deploy       全新部署 MSBOOST 节点（会生成新的配置，旧配置失效）
  fetch        从已部署的节点取回配置，不改动服务器
  cleanup      预览并卸载本工具装过的组件（msboost 为节点，relay 为中转）

登录用户固定为 root。不填 -key 时用密码登录；密码和私钥口令在运行后输入，不显示在屏幕上。
`

func main() {
	if len(os.Args) < 2 {
		fmt.Printf(usage, version)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	var err error
	switch os.Args[1] {
	case "fingerprint":
		err = showFingerprint(ctx, os.Args[2:])
	case "deploy", "fetch", "cleanup":
		err = runTask(ctx, os.Args[1], os.Args[2:])
	default:
		fmt.Printf(usage, version)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误：", err)
		os.Exit(1)
	}
}

func showFingerprint(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("fingerprint", flag.ContinueOnError)
	host := fs.String("host", "", "服务器公网 IP")
	port := fs.Int("port", 22, "SSH 端口")
	if err := fs.Parse(args); err != nil {
		return err
	}
	fp, algorithm, err := executor.SSHRemote{}.Probe(ctx, *host, *port)
	if err != nil {
		return err
	}
	fmt.Printf("%s（%s）\n", fp, algorithm)
	return nil
}

func runTask(ctx context.Context, kind string, args []string) error {
	fs := flag.NewFlagSet(kind, flag.ContinueOnError)
	host := fs.String("host", "", "服务器公网 IP")
	port := fs.Int("port", 22, "SSH 端口")
	keyFile := fs.String("key", "", "OpenSSH 格式的私钥文件；不填则用密码登录")
	fingerprint := fs.String("fingerprint", "", "已核对的主机指纹 SHA256:...；不填则先显示指纹再确认")
	out := fs.String("out", "", "保存配置的文件（deploy、fetch）")
	scope := fs.String("scope", "msboost", "清理范围：msboost 或 relay（cleanup）")
	if err := fs.Parse(args); err != nil {
		return err
	}
	s, err := connection(ctx, *host, *port, *keyFile, *fingerprint)
	if err != nil {
		return err
	}
	e := installers.NewEngine()
	switch kind {
	case "deploy":
		fmt.Println("正在部署，通常需要几分钟；中途按 Ctrl+C 只会断开本机连接。")
		taskCtx, cancel := context.WithTimeout(ctx, 45*time.Minute)
		defer cancel()
		r := e.Execute(taskCtx, executor.Job{ID: newID(), Request: executor.Request{Kind: "deploy", SSH: s, Mode: "fresh"}, Script: installers.NodeScript()})
		report(r)
		return saveConfig(r, *out, s.Host)
	case "fetch":
		taskCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		r := e.FetchConfig(taskCtx, s)
		report(r)
		return saveConfig(r, *out, s.Host)
	default:
		return cleanup(ctx, e, s, *scope)
	}
}

// connection collects the SSH identity: the host key (shown and confirmed when
// not given) and the password or private key, read without echo.
func connection(ctx context.Context, host string, port int, keyFile, fingerprint string) (executor.SSH, error) {
	s := executor.SSH{Host: host, Port: port, User: "root", Fingerprint: fingerprint, TrustMode: "strict"}
	if err := executor.PublicIP(host); err != nil {
		return s, err
	}
	if fingerprint == "" {
		fp, algorithm, err := executor.SSHRemote{}.Probe(ctx, host, port)
		if err != nil {
			return s, err
		}
		fmt.Printf("服务器 %s 的 SSH 指纹：%s（%s）\n", host, fp, algorithm)
		fmt.Println("首次连接无法排除中间人攻击，最好先在 VPS 服务商控制台核对这个指纹。")
		if !confirm("信任这台服务器并继续？输入 yes：") {
			return s, errors.New("已取消")
		}
		s.Fingerprint, s.TrustMode = fp, "tofu"
	}
	if keyFile == "" {
		password, err := readSecret("root 密码：")
		if err != nil {
			return s, err
		}
		s.Password = password
		return s, executor.ValidateSSH(s)
	}
	key, err := os.ReadFile(keyFile)
	if err != nil {
		return s, err
	}
	s.AuthMethod, s.PrivateKey = "key", string(key)
	var missing *ssh.PassphraseMissingError
	if _, err := ssh.ParsePrivateKey(key); errors.As(err, &missing) {
		if s.Passphrase, err = readSecret("私钥口令："); err != nil {
			return s, err
		}
	}
	return s, executor.ValidateSSH(s)
}

func cleanup(ctx context.Context, e *executor.Engine, s executor.SSH, scope string) error {
	previewCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	preview := e.Execute(previewCtx, executor.Job{ID: newID(), Request: executor.Request{Kind: "cleanup-preview", SSH: s, Cleanup: &executor.CleanupOptions{Scope: scope}}})
	if preview.State != "succeeded" {
		report(preview)
		return errors.New("预览失败，没有删除任何内容")
	}
	if len(preview.Cleanup.Items) == 0 {
		fmt.Println("没有需要清理的内容。")
		return nil
	}
	fmt.Println("将删除以下内容：")
	for _, item := range preview.Cleanup.Items {
		fmt.Printf("  [%s] %s\n", item.Kind, item.Path)
	}
	if !confirm("确认删除请输入 yes：") {
		return errors.New("已取消，没有删除任何内容")
	}
	cleanupCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	r := e.Execute(cleanupCtx, executor.Job{ID: newID(), Request: executor.Request{Kind: "cleanup", SSH: s, Cleanup: &executor.CleanupOptions{Scope: scope, PreviewID: preview.ID, Digest: preview.Cleanup.Digest, Confirm: true}}})
	report(r)
	if r.State != "succeeded" {
		return errors.New("清理未完成")
	}
	return nil
}

func report(r executor.Result) {
	fmt.Printf("结果：%s（阶段 %s）\n", r.State, r.Phase)
	if r.Message != "" {
		fmt.Println(r.Message)
	}
	if r.ErrorCode != "" {
		fmt.Println("诊断编号：" + r.ErrorCode)
	}
	if r.NextStep != "" {
		fmt.Println("建议：" + r.NextStep)
	}
	if r.ErrorCode == "task_timeout" {
		fmt.Println("本机已断开连接，服务器上的安装可能仍在继续；稍后用 fetch 取回配置确认。")
	}
	if h := r.Health; h != nil {
		var parts []string
		for _, f := range [][2]string{{"服务", h.Service}, {"本地自测", h.LocalSelfTest}, {"公网 TCP", h.PublicTCP}, {"BBR", h.BBR}} {
			if f[1] != "" {
				parts = append(parts, f[0]+"："+f[1])
			}
		}
		fmt.Println(strings.Join(parts, "　"))
	}
}

func saveConfig(r executor.Result, out, host string) error {
	if r.State != "succeeded" || len(r.Config) == 0 {
		return errors.New("没有取得配置")
	}
	if out == "" {
		out = "直连-" + host + ".json"
	}
	if _, err := os.Stat(out); err == nil && !confirm(out+" 已存在，覆盖请输入 yes：") {
		return errors.New("配置没有保存")
	}
	if err := os.WriteFile(out, r.Config, 0o600); err != nil {
		return err
	}
	fmt.Println("配置已保存到 " + out + "，放进 MSBOOST 启动器的 configs 文件夹即可使用。")
	return nil
}

func readSecret(prompt string) (string, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return "", errors.New("请在 PowerShell 或命令提示符窗口里运行，才能不回显地输入密码")
	}
	fmt.Print(prompt)
	secret, err := term.ReadPassword(fd)
	fmt.Println()
	return string(secret), err
}

func confirm(prompt string) bool {
	fmt.Print(prompt)
	line, _ := stdin.ReadString('\n')
	return strings.TrimSpace(line) == "yes"
}

func newID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
