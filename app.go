package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	xwindows "golang.org/x/sys/windows"

	"github.com/mozziexwz/node_local/internal/executor"
	"github.com/mozziexwz/node_local/internal/logx"
	"github.com/mozziexwz/node_local/internal/servers"
	"github.com/mozziexwz/node_local/internal/store"
	"github.com/mozziexwz/node_local/internal/tasks"
)

// App is bound to the frontend; its exported methods are the UI's API.
type App struct {
	ctx      context.Context
	layout   store.Layout
	log      *logx.Logger
	servers  *servers.Store
	runner   *tasks.Runner
	warnings []string // startup problems, shown as banners

	mu       sync.Mutex
	settings store.Settings
}

func NewApp(layout store.Layout, log *logx.Logger) *App {
	a := &App{layout: layout, log: log}
	var err error
	if a.settings, err = layout.LoadSettings(); err != nil {
		a.warnings = append(a.warnings, err.Error())
	}
	if a.servers, err = servers.Open(layout.Servers); err != nil {
		a.warnings = append(a.warnings, err.Error())
	}
	var problems []error
	a.runner, problems = tasks.New(layout, a.servers, log, a.currentSettings, a.emitTask)
	for _, p := range problems {
		a.warnings = append(a.warnings, p.Error())
	}
	return a
}

func (a *App) currentSettings() store.Settings {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.settings
}

func (a *App) emitTask(e tasks.Event) {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "task", e)
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.log.Printf(logx.App, "MSBOOST 本地版已启动，版本 %s，数据目录 %s", versionString(), a.layout.Root)
}

// beforeClose asks first while a task runs: closing disconnects it.
func (a *App) beforeClose(ctx context.Context) bool {
	if !a.runner.Busy() {
		return false
	}
	answer, err := runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
		Type:    runtime.QuestionDialog,
		Title:   "有操作正在进行",
		Message: "关闭程序会断开连接，服务器上的操作可能还会继续。确定关闭吗？",
	})
	return err != nil || (answer != "Yes" && answer != "是")
}

func (a *App) shutdown(context.Context) {
	a.log.Printf(logx.App, "MSBOOST 本地版已退出")
	a.log.Close()
}

func (a *App) onSecondInstance(options.SecondInstanceData) {
	runtime.WindowUnminimise(a.ctx)
	runtime.WindowShow(a.ctx)
}

// ---- overview ----

// Overview returns everything the pages need in one call.
func (a *App) Overview() OverviewDTO {
	settings := a.currentSettings()
	dto := OverviewDTO{
		Running:    a.runner.Running(),
		Proxy:      settings.Proxy,
		ConfigsDir: a.layout.ConfigsDir(settings),
		DataDir:    a.layout.Local,
		Version:    versionString(),
		Warnings:   a.warnings,
	}
	launcher, found := a.layout.FindLauncher(settings)
	if found {
		dto.Launcher = &launcher
	}
	for _, sv := range a.servers.List() {
		dto.Servers = append(dto.Servers, toServerDTO(sv, dto.ConfigsDir, found))
	}
	return dto
}

// ---- servers ----

// ProbeHost reads a server's host key without logging in.
func (a *App) ProbeHost(host string, port int) (ProbeDTO, error) {
	host = strings.TrimSpace(host)
	if err := executor.PublicIP(host); err != nil {
		return ProbeDTO{}, err
	}
	fingerprint, algorithm, err := a.runner.Probe(host, port)
	if err != nil {
		return ProbeDTO{}, err
	}
	return ProbeDTO{Fingerprint: fingerprint, Algorithm: algorithm}, nil
}

// AddServer saves a server whose host key the user has just trusted.
func (a *App) AddServer(in ServerInput) (ServerDTO, error) {
	sv, err := a.servers.Add(servers.Server{Name: strings.TrimSpace(in.Name), Host: strings.TrimSpace(in.Host), Port: in.Port,
		AuthMethod: in.AuthMethod, KeyPath: in.KeyPath, Fingerprint: in.Fingerprint, Algorithm: in.Algorithm})
	if err != nil {
		return ServerDTO{}, err
	}
	a.log.Printf(logx.App, "已添加服务器 %s（%s），指纹 %s", sv.Label(), sv.Address(), sv.Fingerprint)
	return a.serverDTO(sv), nil
}

// UpdateServer changes the remark, SSH port or login method. The host key
// stays; a different key is replaced only through ReplaceFingerprint.
func (a *App) UpdateServer(id string, in ServerInput) (ServerDTO, error) {
	if a.busy(id) {
		return ServerDTO{}, errors.New("这台服务器上有操作正在进行，请等它结束")
	}
	sv, err := a.servers.Update(id, func(v *servers.Server) error {
		v.Name, v.Port, v.AuthMethod, v.KeyPath = strings.TrimSpace(in.Name), in.Port, in.AuthMethod, in.KeyPath
		return nil
	})
	if err != nil {
		return ServerDTO{}, err
	}
	// The login may have changed, so a remembered secret may no longer fit.
	a.runner.ForgetSession(id)
	return a.serverDTO(sv), nil
}

// ReplaceFingerprint trusts a server's new host key after the user checked
// why it changed.
func (a *App) ReplaceFingerprint(id, fingerprint, algorithm string) (ServerDTO, error) {
	var old string
	sv, err := a.servers.Update(id, func(v *servers.Server) error {
		old = v.Fingerprint
		v.Fingerprint, v.Algorithm, v.TrustedAt = fingerprint, algorithm, time.Now().UnixMilli()
		return nil
	})
	if err != nil {
		return ServerDTO{}, err
	}
	a.runner.ForgetSession(id)
	a.log.Printf(logx.App, "已替换 %s 的服务器指纹：%s → %s", sv.Label(), old, fingerprint)
	return a.serverDTO(sv), nil
}

// RemoveServer forgets a server. Nothing on the server changes.
func (a *App) RemoveServer(id string) error {
	if a.busy(id) {
		return errors.New("这台服务器上有操作正在进行，请等它结束")
	}
	sv, _ := a.servers.Get(id)
	if err := a.servers.Remove(id); err != nil {
		return err
	}
	a.runner.ForgetRelays(id)
	a.log.Printf(logx.App, "已从列表移除服务器 %s（%s）", sv.Label(), sv.Address())
	return nil
}

func (a *App) busy(serverID string) bool {
	for _, e := range a.runner.Running() {
		if e.ServerID == serverID {
			return true
		}
	}
	return false
}

// PickKeyFile asks for an OpenSSH private key; "" means cancelled.
func (a *App) PickKeyFile() (string, error) {
	opts := runtime.OpenDialogOptions{Title: "选择 SSH 私钥文件（OpenSSH 格式）"}
	if home, err := os.UserHomeDir(); err == nil {
		if dir := filepath.Join(home, ".ssh"); isDir(dir) {
			opts.DefaultDirectory = dir
		}
	}
	return runtime.OpenFileDialog(a.ctx, opts)
}

// ---- tasks ----

// SecretNeeded says what to ask before an operation of this kind:
// "password", "passphrase", or "" when nothing is needed.
func (a *App) SecretNeeded(serverID, kind string) (string, error) {
	return a.runner.SecretNeeded(serverID, tasks.Kind(kind))
}

// StartTask begins an operation; see tasks.Kind.
func (a *App) StartTask(kind, serverID, password, passphrase string, opts TaskOptions) (tasks.Event, error) {
	o := tasks.Options{FrontID: opts.FrontID, FrontSecret: tasks.Secret{Password: opts.FrontPassword, Passphrase: opts.FrontPassphrase},
		Remark: strings.TrimSpace(opts.Remark), RateMbps: opts.RateMbps, Target: opts.Target, Action: opts.Action, Items: opts.Items}
	if opts.ConfigName != "" {
		path, err := a.configPath(opts.ConfigName)
		if err != nil {
			return tasks.Event{}, err
		}
		o.ConfigPath = path
	}
	return a.runner.Start(tasks.Kind(kind), serverID, tasks.Secret{Password: password, Passphrase: passphrase}, o)
}

// ---- configurations and friend relays ----

// Configs lists the valid node configurations where new ones are saved.
func (a *App) Configs() []ConfigDTO {
	dir := a.layout.ConfigsDir(a.currentSettings())
	entries, _ := os.ReadDir(dir)
	out := []ConfigDTO{}
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		if entry, err := tasks.NodeEntry(data); err == nil {
			out = append(out, ConfigDTO{Name: e.Name(), Entry: entry})
		}
	}
	return out
}

func (a *App) configPath(name string) (string, error) {
	if name != filepath.Base(name) || strings.ContainsAny(name, `/\`) || !strings.EqualFold(filepath.Ext(name), ".json") {
		return "", fmt.Errorf("配置文件名无效：%q", name)
	}
	return filepath.Join(a.layout.ConfigsDir(a.currentSettings()), name), nil
}

func (a *App) readConfig(name string) ([]byte, error) {
	path, err := a.configPath(name)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("无法读取配置 %s：%w", name, err)
	}
	return data, nil
}

// ForwardRequest is the text a node's owner sends to the friend who relays:
// the node's address, nothing else.
func (a *App) ForwardRequest(configName string) (string, error) {
	data, err := a.readConfig(configName)
	if err != nil {
		return "", err
	}
	node, err := tasks.NodeEntry(data)
	if err != nil {
		return "", err
	}
	return tasks.ShareRequest(node), nil
}

// CheckFriendEntry reads the text a friend sent back, compares it with the
// configuration's node and tests the entry from this computer.
func (a *App) CheckFriendEntry(configName, text string) (FriendCheckDTO, error) {
	data, err := a.readConfig(configName)
	if err != nil {
		return FriendCheckDTO{}, err
	}
	c, err := tasks.CheckFriendEntry(data, text)
	if err != nil {
		return FriendCheckDTO{}, err
	}
	reachable := "unreachable"
	if conn, err := net.DialTimeout("tcp", c.Entry, 6*time.Second); err == nil {
		conn.Close()
		reachable = "reachable"
	}
	return FriendCheckDTO{Entry: c.Entry, Target: c.Target, Node: c.Node, Matches: c.Matches, Reachable: reachable}, nil
}

// SaveFriendConfig writes a new configuration that reaches the node through
// the friend's relay; the original stays as it was. It returns the path.
func (a *App) SaveFriendConfig(configName, text, name string) (string, error) {
	data, err := a.readConfig(configName)
	if err != nil {
		return "", err
	}
	name = strings.TrimSpace(name)
	out, err := tasks.FriendConfig(data, text, name)
	if err != nil {
		return "", err
	}
	if name == "" {
		name = strings.TrimSuffix(configName, filepath.Ext(configName)) + "-经朋友中转"
	}
	dir := a.layout.ConfigsDir(a.currentSettings())
	path, err := store.SaveConfig(dir, store.FreeConfigName(dir, store.ConfigName(name), ""), out)
	if err != nil {
		return "", err
	}
	a.log.Printf(logx.App, "已生成经朋友中转的配置：%s", path)
	return path, nil
}

// ConfirmUninstall removes what the preview listed.
func (a *App) ConfirmUninstall(previewID string) (tasks.Event, error) {
	return a.runner.ConfirmUninstall(previewID)
}

// DiscardUninstall drops an unconfirmed preview.
func (a *App) DiscardUninstall(previewID string) { a.runner.DiscardUninstall(previewID) }

// CancelTask disconnects a running task.
func (a *App) CancelTask(taskID string) { a.runner.Cancel(taskID) }

// Records lists finished operations, newest first.
func (a *App) Records() []tasks.Record { return a.runner.Records() }

// ClearRecords empties the task records.
func (a *App) ClearRecords() error { return a.runner.ClearRecords() }

// ---- settings ----

// SaveProxy changes the SSH proxy setting.
func (a *App) SaveProxy(enabled bool, host string, port int) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	s := a.settings
	s.Proxy = store.Proxy{Enabled: enabled, Host: strings.TrimSpace(host), Port: port}
	if err := a.layout.SaveSettings(s); err != nil {
		return err
	}
	a.settings = s
	a.log.Printf(logx.App, "SSH 代理：%v %s", enabled, s.Proxy.Address())
	return nil
}

// PickLauncher asks where MSBOOST-Launcher.exe is and remembers its folder;
// false means cancelled.
func (a *App) PickLauncher() (bool, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:   "选择 MSBOOST 启动器（" + store.LauncherExe + "）",
		Filters: []runtime.FileFilter{{DisplayName: "MSBOOST 启动器", Pattern: store.LauncherExe}},
	})
	if err != nil || path == "" {
		return false, err
	}
	if !strings.EqualFold(filepath.Base(path), store.LauncherExe) {
		return false, fmt.Errorf("请选择 %s", store.LauncherExe)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	s := a.settings
	s.LauncherDir = filepath.Dir(path)
	if err := a.layout.SaveSettings(s); err != nil {
		return false, err
	}
	a.settings = s
	a.log.Printf(logx.App, "启动器位置：%s", s.LauncherDir)
	return true, nil
}

// ForgetLauncher clears the saved launcher folder.
func (a *App) ForgetLauncher() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	s := a.settings
	s.LauncherDir = ""
	if err := a.layout.SaveSettings(s); err != nil {
		return err
	}
	a.settings = s
	return nil
}

// CopyToLauncher copies a server's configuration, saved before the launcher
// was known, into the launcher's configs\ and returns the new path.
func (a *App) CopyToLauncher(serverID string) (string, error) {
	sv, ok := a.servers.Get(serverID)
	if !ok || sv.ConfigFile == "" {
		return "", errors.New("这台服务器还没有配置文件")
	}
	settings := a.currentSettings()
	if _, found := a.layout.FindLauncher(settings); !found {
		return "", errors.New("还不知道启动器在哪里")
	}
	data, err := os.ReadFile(filepath.Join(sv.ConfigDir, sv.ConfigFile))
	if err != nil {
		return "", fmt.Errorf("无法读取配置文件：%w", err)
	}
	dir := a.layout.ConfigsDir(settings)
	path, err := store.SaveConfig(dir, sv.ConfigFile, data)
	if err != nil {
		return "", err
	}
	if _, err := a.servers.Update(serverID, func(v *servers.Server) error { v.ConfigDir = dir; return nil }); err != nil {
		return "", err
	}
	a.log.Printf(logx.App, "已把 %s 复制到启动器：%s", sv.ConfigFile, path)
	return path, nil
}

// OpenFolder opens configs, data or logs in Explorer.
func (a *App) OpenFolder(kind string) error {
	dirs := map[string]string{"configs": a.layout.ConfigsDir(a.currentSettings()), "data": a.layout.Local, "logs": a.layout.Logs}
	dir, ok := dirs[kind]
	if !ok {
		return fmt.Errorf("未知的文件夹：%q", kind)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return exec.Command("explorer.exe", dir).Start()
}

// OpenLauncher starts the launcher. It asks for administrator rights itself,
// so it is started through the shell, which shows the UAC prompt.
func (a *App) OpenLauncher() error {
	launcher, found := a.layout.FindLauncher(a.currentSettings())
	if !found {
		return errors.New("还不知道启动器在哪里")
	}
	exe := filepath.Join(launcher.Dir, store.LauncherExe)
	verb, _ := xwindows.UTF16PtrFromString("open")
	file, _ := xwindows.UTF16PtrFromString(exe)
	dir, _ := xwindows.UTF16PtrFromString(launcher.Dir)
	err := xwindows.ShellExecute(0, verb, file, nil, dir, xwindows.SW_SHOWNORMAL)
	if errors.Is(err, xwindows.ERROR_CANCELLED) {
		return nil // the user declined the administrator prompt
	}
	return err
}

// About returns version and license information.
func (a *App) About() AboutDTO { return aboutInfo() }

// OpenURL opens an https link in the default browser.
func (a *App) OpenURL(url string) {
	if strings.HasPrefix(url, "https://") {
		runtime.BrowserOpenURL(a.ctx, url)
	}
}

func (a *App) serverDTO(sv servers.Server) ServerDTO {
	settings := a.currentSettings()
	_, found := a.layout.FindLauncher(settings)
	return toServerDTO(sv, a.layout.ConfigsDir(settings), found)
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
