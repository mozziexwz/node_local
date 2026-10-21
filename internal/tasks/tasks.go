// Package tasks runs the operations on servers: at most one at a time per
// server, with progress events for the window and a record of each finished
// operation in local\tasks.json. Passwords and passphrases live only in
// memory for one operation; records never hold them or configuration
// contents.
package tasks

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/net/proxy"

	"github.com/mozziexwz/node_local/installers"
	"github.com/mozziexwz/node_local/internal/executor"
	"github.com/mozziexwz/node_local/internal/logx"
	"github.com/mozziexwz/node_local/internal/servers"
	"github.com/mozziexwz/node_local/internal/store"
)

// Kind is an operation on a server.
type Kind string

const (
	Deploy           Kind = "deploy"            // fresh node, new configuration
	Upgrade          Kind = "upgrade"           // newer installer, same configuration
	Fetch            Kind = "fetch"             // read the configuration back
	Status           Kind = "status"            // read-only look at the server
	UninstallPreview Kind = "uninstall-preview" // list what uninstalling removes
	Uninstall        Kind = "uninstall"         // remove the node
)

var kindLabels = map[Kind]string{
	Deploy: "部署节点", Upgrade: "升级节点", Fetch: "取回配置", Status: "刷新状态",
	UninstallPreview: "读取卸载清单", Uninstall: "卸载节点",
}

// Label is the operation's name in Chinese.
func (k Kind) Label() string { return kindLabels[k] }

// Status checks and previews change nothing, so they are not recorded.
func (k Kind) recorded() bool { return k == Deploy || k == Upgrade || k == Fetch || k == Uninstall }

func (k Kind) timeout() time.Duration {
	switch k {
	case Deploy, Upgrade:
		return 45 * time.Minute
	case Uninstall, UninstallPreview:
		return 5 * time.Minute
	}
	return 2 * time.Minute
}

// Secret is what the user typed for one operation.
type Secret struct {
	Password   string
	Passphrase string
}

// Event tells the window about a task. Step is an executor.Step while a
// deploy or upgrade runs.
type Event struct {
	ID       string   `json:"id"`
	ServerID string   `json:"serverId"`
	Kind     Kind     `json:"kind"`
	State    string   `json:"state"` // running, succeeded, failed or cancelled
	Step     int      `json:"step"`
	Started  int64    `json:"started"`
	Finished int64    `json:"finished,omitempty"`
	Outcome  *Outcome `json:"outcome,omitempty"`
}

// Outcome is a finished task's result.
type Outcome struct {
	Phase      string                 `json:"phase,omitempty"`
	ErrorCode  string                 `json:"errorCode,omitempty"`
	Message    string                 `json:"message,omitempty"`
	NextStep   string                 `json:"nextStep,omitempty"`
	Health     *executor.Health       `json:"health,omitempty"`
	ConfigFile string                 `json:"configFile,omitempty"`
	ConfigPath string                 `json:"configPath,omitempty"`
	Items      []executor.CleanupItem `json:"items,omitempty"` // what an uninstall removes or removed
	Node       *servers.NodeState     `json:"node,omitempty"`
}

// Record is one finished operation in local\tasks.json.
type Record struct {
	ID          string  `json:"id"`
	ServerID    string  `json:"serverId"`
	Server      string  `json:"server"`
	Address     string  `json:"address"`
	Fingerprint string  `json:"fingerprint"`
	Kind        Kind    `json:"kind"`
	State       string  `json:"state"`
	Started     int64   `json:"started"`
	Finished    int64   `json:"finished"`
	Outcome     Outcome `json:"outcome"`
}

const maxRecords = 200

type job struct {
	event     Event
	cancel    context.CancelFunc
	cancelled atomic.Bool
}

// pendingUninstall holds a preview until the user confirms it. The secret is
// kept so the user need not type it twice for one uninstall.
type pendingUninstall struct {
	serverID  string
	previewID string
	digest    string
	secret    Secret
	expires   time.Time
}

const confirmWindow = 10 * time.Minute

// Runner runs tasks. It is safe for concurrent use.
type Runner struct {
	layout   store.Layout
	servers  *servers.Store
	log      *logx.Logger
	settings func() store.Settings
	emit     func(Event)

	// Test seams.
	newEngine func(store.Settings) *executor.Engine
	readKey   func(string) ([]byte, error)
	now       func() time.Time

	mu      sync.Mutex
	running map[string]*job // by server ID
	pending map[string]pendingUninstall
	records []Record
}

// New returns a runner. emit receives every event; it must not block.
func New(layout store.Layout, list *servers.Store, log *logx.Logger, settings func() store.Settings, emit func(Event)) (*Runner, error) {
	r := &Runner{layout: layout, servers: list, log: log, settings: settings, emit: emit,
		newEngine: defaultEngine, readKey: os.ReadFile, now: time.Now,
		running: map[string]*job{}, pending: map[string]pendingUninstall{}}
	var err error
	r.records, err = loadRecords(layout.Tasks)
	return r, err
}

// defaultEngine has the pinned GOST archives and dials through the SOCKS5
// proxy when the settings ask for it.
func defaultEngine(s store.Settings) *executor.Engine {
	e := installers.NewEngine()
	if s.Proxy.Enabled {
		e.Remote = executor.NewSSHRemote(proxyDialer(s.Proxy.Address()))
	}
	return e
}

func proxyDialer(address string) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, target string) (net.Conn, error) {
		ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		d, err := proxy.SOCKS5("tcp", address, nil, &net.Dialer{Timeout: 15 * time.Second})
		if err != nil {
			return nil, err
		}
		return d.(proxy.ContextDialer).DialContext(ctx, network, target)
	}
}

// Probe reads a server's host key without logging in.
func (r *Runner) Probe(host string, port int) (fingerprint, algorithm string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	settings := r.settings()
	fingerprint, algorithm, err = r.newEngine(settings).Remote.Probe(ctx, host, port)
	var d executor.Diagnostic
	if errors.As(err, &d) {
		o := Outcome{ErrorCode: d.Code, Message: d.Message, NextStep: d.NextStep}
		localize(&o, settings)
		return "", "", errors.New(o.Message + "。" + o.NextStep)
	}
	return fingerprint, algorithm, err
}

// SecretNeeded says what to ask before an operation on the server:
// "password", "passphrase" for an encrypted key, or "" for a key without one.
func (r *Runner) SecretNeeded(serverID string) (string, error) {
	sv, ok := r.servers.Get(serverID)
	if !ok {
		return "", errors.New("找不到这台服务器")
	}
	if sv.AuthMethod != "key" {
		return "password", nil
	}
	key, err := r.readKey(sv.KeyPath)
	if err != nil {
		return "", fmt.Errorf("无法读取私钥文件 %s：%v", sv.KeyPath, err)
	}
	_, err = ssh.ParsePrivateKey(key)
	var missing *ssh.PassphraseMissingError
	if errors.As(err, &missing) {
		return "passphrase", nil
	}
	if _, err := executor.ParsePrivateKey(string(key), ""); err != nil {
		return "", err
	}
	return "", nil
}

// connection is the SSH identity for one operation.
func (r *Runner) connection(sv servers.Server, secret Secret) (executor.SSH, error) {
	s := executor.SSH{Host: sv.Host, Port: sv.Port, User: "root", Fingerprint: sv.Fingerprint, TrustMode: "tofu"}
	if sv.AuthMethod == "key" {
		key, err := r.readKey(sv.KeyPath)
		if err != nil {
			return s, fmt.Errorf("无法读取私钥文件 %s：%v", sv.KeyPath, err)
		}
		s.AuthMethod, s.PrivateKey, s.Passphrase = "key", string(key), secret.Passphrase
	} else {
		s.AuthMethod, s.Password = "password", secret.Password
	}
	if err := executor.ValidateSSH(s); err != nil {
		return s, err
	}
	return s, nil
}

// Start begins an operation; uninstalling starts from UninstallPreview and
// ConfirmUninstall.
func (r *Runner) Start(kind Kind, serverID string, secret Secret) (Event, error) {
	switch kind {
	case Deploy, Upgrade, Fetch, Status, UninstallPreview:
	default:
		return Event{}, errors.New("不支持的操作")
	}
	sv, ok := r.servers.Get(serverID)
	if !ok {
		return Event{}, errors.New("找不到这台服务器")
	}
	conn, err := r.connection(sv, secret)
	if err != nil {
		return Event{}, err
	}
	return r.launch(kind, sv, conn, secret, nil)
}

// ConfirmUninstall removes what the preview listed. The server refuses if
// the list changed since.
func (r *Runner) ConfirmUninstall(previewID string) (Event, error) {
	r.mu.Lock()
	p, ok := r.pending[previewID]
	delete(r.pending, previewID)
	r.sweepLocked()
	r.mu.Unlock()
	if !ok || r.now().After(p.expires) {
		return Event{}, errors.New("卸载确认已过期，请重新读取卸载清单")
	}
	sv, ok := r.servers.Get(p.serverID)
	if !ok {
		return Event{}, errors.New("找不到这台服务器")
	}
	conn, err := r.connection(sv, p.secret)
	if err != nil {
		return Event{}, err
	}
	return r.launch(Uninstall, sv, conn, p.secret, &p)
}

// DiscardUninstall drops a preview the user did not confirm.
func (r *Runner) DiscardUninstall(previewID string) {
	r.mu.Lock()
	delete(r.pending, previewID)
	r.sweepLocked()
	r.mu.Unlock()
}

func (r *Runner) sweepLocked() {
	now := r.now()
	for id, p := range r.pending {
		if now.After(p.expires) {
			delete(r.pending, id)
		}
	}
}

// Cancel disconnects a running task. What already runs on the server may
// carry on there.
func (r *Runner) Cancel(taskID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, j := range r.running {
		if j.event.ID == taskID {
			j.cancelled.Store(true)
			j.cancel()
		}
	}
}

// Running lists the tasks in progress.
func (r *Runner) Running() []Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Event, 0, len(r.running))
	for _, j := range r.running {
		out = append(out, j.event)
	}
	return out
}

// Busy reports whether any task is running.
func (r *Runner) Busy() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.running) > 0
}

// Records lists finished operations, newest first.
func (r *Runner) Records() []Record {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Record(nil), r.records...)
}

// ClearRecords empties local\tasks.json.
func (r *Runner) ClearRecords() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := store.WriteJSON(r.layout.Tasks, recordsFile{Version: 1}); err != nil {
		return err
	}
	r.records = nil
	return nil
}

func (r *Runner) launch(kind Kind, sv servers.Server, conn executor.SSH, secret Secret, confirm *pendingUninstall) (Event, error) {
	r.mu.Lock()
	if _, busy := r.running[sv.ID]; busy {
		r.mu.Unlock()
		return Event{}, errors.New("这台服务器上有操作正在进行，请等它结束")
	}
	ctx, cancel := context.WithTimeout(context.Background(), kind.timeout())
	j := &job{event: Event{ID: newID(), ServerID: sv.ID, Kind: kind, State: "running", Started: r.now().UnixMilli()}, cancel: cancel}
	r.running[sv.ID] = j
	ev := j.event
	r.mu.Unlock()
	r.emit(ev)
	go r.run(ctx, j, sv, conn, secret, confirm)
	return ev, nil
}

func (r *Runner) run(ctx context.Context, j *job, sv servers.Server, conn executor.SSH, secret Secret, confirm *pendingUninstall) {
	defer j.cancel()
	for _, s := range []string{secret.Password, secret.Passphrase} {
		if s != "" {
			r.log.AddSecret(s)
			defer r.log.RemoveSecret(s)
		}
	}
	kind := j.event.Kind
	settings := r.settings()
	e := r.newEngine(settings)
	progress := func(step executor.Step) {
		r.mu.Lock()
		if int(step) <= j.event.Step {
			r.mu.Unlock()
			return
		}
		j.event.Step = int(step)
		ev := j.event
		r.mu.Unlock()
		r.emit(ev)
	}
	if kind != Status {
		r.log.Printf(logx.Task, "开始%s：%s（%s）", kind.Label(), sv.Label(), sv.Address())
	}

	var out Outcome
	var digest string
	state := "failed"
	task := executor.Job{ID: j.event.ID, Script: installers.NodeScript()}
	switch kind {
	case Deploy:
		task.Request = executor.Request{Kind: "deploy", Mode: "fresh", SSH: conn}
		state, out = r.finishConfig(sv, e.DeployWithProgress(ctx, task, progress), settings)
	case Upgrade:
		task.Request = executor.Request{SSH: conn}
		state, out = r.finishConfig(sv, e.UpgradeWithProgress(ctx, task, progress), settings)
	case Fetch:
		state, out = r.finishConfig(sv, e.FetchConfig(ctx, conn), settings)
	case Status:
		st, res := e.NodeStatus(ctx, conn)
		state, out = r.finishStatus(sv, st, res)
	case UninstallPreview:
		task.Request = executor.Request{Kind: "cleanup-preview", SSH: conn, Cleanup: &executor.CleanupOptions{Scope: "msboost"}}
		state, out, digest = finishPreview(e.Execute(ctx, task))
	case Uninstall:
		task.Request = executor.Request{Kind: "cleanup", SSH: conn, Cleanup: &executor.CleanupOptions{Scope: "msboost", PreviewID: confirm.previewID, Digest: confirm.digest, Confirm: true}}
		state, out = r.finishUninstall(sv, e.Execute(ctx, task))
	}
	if state != "succeeded" && j.cancelled.Load() {
		state = "cancelled"
		out.ErrorCode, out.Phase = "", ""
		out.Message = "已取消：这台电脑断开了连接"
		out.NextStep = "服务器上的操作可能还在继续。稍后用「刷新状态」或「取回配置」确认，不要马上重复部署。"
	}
	localize(&out, settings)

	r.mu.Lock()
	j.event.State, j.event.Finished, j.event.Outcome = state, r.now().UnixMilli(), &out
	ev := j.event
	if r.running[sv.ID] == j {
		delete(r.running, sv.ID)
	}
	if digest != "" {
		r.pending[ev.ID] = pendingUninstall{serverID: sv.ID, previewID: ev.ID, digest: digest, secret: secret, expires: r.now().Add(confirmWindow)}
	}
	if kind.recorded() {
		r.records = append([]Record{{ID: ev.ID, ServerID: sv.ID, Server: sv.Label(), Address: sv.Address(), Fingerprint: sv.Fingerprint,
			Kind: kind, State: state, Started: ev.Started, Finished: ev.Finished, Outcome: out}}, r.records...)
		if len(r.records) > maxRecords {
			r.records = r.records[:maxRecords]
		}
		if err := store.WriteJSON(r.layout.Tasks, recordsFile{Version: 1, Records: r.records}); err != nil {
			r.log.Printf(logx.Task, "无法保存任务记录：%v", err)
		}
	}
	r.mu.Unlock()
	if kind != Status {
		if state == "succeeded" {
			r.log.Printf(logx.Task, "%s完成：%s", kind.Label(), sv.Label())
		} else {
			r.log.Printf(logx.Task, "%s未完成：%s，%s", kind.Label(), sv.Label(), out.Message)
		}
	}
	r.emit(ev)
}

// failure turns an engine result into an outcome.
func failure(res executor.Result) Outcome {
	return Outcome{Phase: res.Phase, ErrorCode: res.ErrorCode, Message: res.Message, NextStep: res.NextStep, Health: res.Health}
}

// finishConfig saves a deploy, upgrade or fetch result's configuration where
// the launcher reads it, and remembers it on the server.
func (r *Runner) finishConfig(sv servers.Server, res executor.Result, settings store.Settings) (string, Outcome) {
	if res.State != "succeeded" {
		return "failed", failure(res)
	}
	info, err := executor.ParseClientConfig(res.Config)
	if err != nil {
		return "failed", Outcome{ErrorCode: "invalid_config", Message: "节点配置格式不符合要求"}
	}
	dir := r.layout.ConfigsDir(settings)
	own := ""
	if sv.ConfigDir == dir {
		own = sv.ConfigFile
	}
	name := store.FreeConfigName(dir, store.ConfigName(sv.Label()), own)
	path, err := store.SaveConfig(dir, name, res.Config)
	if err != nil {
		return "failed", Outcome{Phase: "config", ErrorCode: "config_not_saved", Message: err.Error(),
			NextStep: "节点已经就绪。腾出磁盘空间或换一个可写的文件夹后，用「取回配置」重新保存。"}
	}
	node := &servers.NodeState{Installed: true, Service: "running", Entry: net.JoinHostPort(info.TargetHost, strconv.Itoa(info.TargetPort)), CheckedAt: r.now().UnixMilli()}
	if h := res.Health; h != nil {
		if h.Service != "" {
			node.Service = h.Service
		}
		node.BBR, node.PublicTCP = h.BBR, h.PublicTCP
	}
	if sv.Node != nil {
		node.Engine, node.OS = sv.Node.Engine, sv.Node.OS
		if node.BBR == "" {
			node.BBR = sv.Node.BBR
		}
	}
	if _, err := r.servers.Update(sv.ID, func(v *servers.Server) error {
		v.ConfigFile, v.ConfigDir, v.Node = name, dir, node
		return nil
	}); err != nil {
		r.log.Printf(logx.Task, "无法更新服务器记录：%v", err)
	}
	return "succeeded", Outcome{Phase: res.Phase, Message: res.Message, Health: res.Health, ConfigFile: name, ConfigPath: path, Node: node}
}

func (r *Runner) finishStatus(sv servers.Server, st executor.NodeStatus, res executor.Result) (string, Outcome) {
	if res.State != "succeeded" {
		return "failed", failure(res)
	}
	node := &servers.NodeState{Installed: st.Installed, Service: st.Service, Engine: st.Engine, BBR: st.BBR, OS: st.OS,
		Entry: st.Entry, PublicTCP: st.PublicTCP, CheckedAt: r.now().UnixMilli()}
	if _, err := r.servers.Update(sv.ID, func(v *servers.Server) error {
		v.Node = node
		return nil
	}); err != nil {
		r.log.Printf(logx.Task, "无法更新服务器记录：%v", err)
	}
	return "succeeded", Outcome{Node: node}
}

// finishPreview also returns the preview's digest, which the confirmation
// must present; it is empty when there is nothing to remove.
func finishPreview(res executor.Result) (string, Outcome, string) {
	if res.State != "succeeded" || res.Cleanup == nil {
		return "failed", failure(res), ""
	}
	if len(res.Cleanup.Items) == 0 {
		return "succeeded", Outcome{Message: "这台服务器上没有本程序装过的节点组件"}, ""
	}
	return "succeeded", Outcome{Items: res.Cleanup.Items}, res.Cleanup.Digest
}

func (r *Runner) finishUninstall(sv servers.Server, res executor.Result) (string, Outcome) {
	if res.State != "succeeded" || res.Cleanup == nil {
		return "failed", failure(res)
	}
	out := Outcome{Items: res.Cleanup.Items, Message: "节点已卸载"}
	if sv.ConfigFile != "" {
		out.Message += "。" + sv.ConfigFile + " 已经失效，可以在启动器里删除"
	}
	node := &servers.NodeState{CheckedAt: r.now().UnixMilli()}
	if sv.Node != nil {
		node.OS, node.BBR = sv.Node.OS, sv.Node.BBR
	}
	if _, err := r.servers.Update(sv.ID, func(v *servers.Server) error {
		v.ConfigFile, v.ConfigDir, v.Node = "", "", node
		return nil
	}); err != nil {
		r.log.Printf(logx.Task, "无法更新服务器记录：%v", err)
	}
	out.Node = node
	return "succeeded", out
}

func newID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
