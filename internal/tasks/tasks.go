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
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/net/proxy"

	"github.com/mozziexwz/node_local/installers"
	"github.com/mozziexwz/node_local/internal/executor"
	"github.com/mozziexwz/node_local/internal/logx"
	"github.com/mozziexwz/node_local/internal/relays"
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
	UninstallPreview Kind = "uninstall-preview" // list what removing the node removes
	Uninstall        Kind = "uninstall"         // remove the node

	Relay                 Kind = "relay"                   // relay to one of the user's nodes
	Forward               Kind = "forward"                 // relay to a friend's node, by address
	RelayList             Kind = "relay-list"              // read the relays on a server
	RelayManage           Kind = "relay-manage"            // pause, resume, restart, limit or delete relays
	RelayUninstallPreview Kind = "relay-uninstall-preview" // list what removing every relay removes
	RelayUninstall        Kind = "relay-uninstall"         // remove every relay
)

var kindLabels = map[Kind]string{
	Deploy: "部署节点", Upgrade: "升级节点", Fetch: "取回配置", Status: "刷新状态",
	UninstallPreview: "读取卸载清单", Uninstall: "卸载节点",
	Relay: "新建自备中转", Forward: "朋友转发", RelayList: "读取中转列表", RelayManage: "管理中转",
	RelayUninstallPreview: "读取中转卸载清单", RelayUninstall: "卸载全部中转",
}

// Label is the operation's name in Chinese.
func (k Kind) Label() string { return kindLabels[k] }

// Reads change nothing, so they are not recorded.
func (k Kind) recorded() bool {
	switch k {
	case Status, UninstallPreview, RelayList, RelayUninstallPreview:
		return false
	}
	return true
}

// Relay management reuses the secret given when the list was read.
func (k Kind) usesSession() bool {
	return k == RelayList || k == RelayManage || k == RelayUninstallPreview
}

func (k Kind) timeout() time.Duration {
	switch k {
	case Deploy, Upgrade:
		return 45 * time.Minute
	case Relay, Forward:
		return 20 * time.Minute
	case Uninstall, UninstallPreview, RelayUninstall, RelayUninstallPreview:
		return 5 * time.Minute
	}
	return 2 * time.Minute
}

// Secret is what the user typed for one operation.
type Secret struct {
	Password   string
	Passphrase string
}

func (s Secret) empty() bool { return s.Password == "" && s.Passphrase == "" }

// Options are a task's parameters beyond the server and its secret.
type Options struct {
	ConfigPath  string   // Relay: the node configuration to relay to
	FrontID     string   // Relay: an optional front server
	FrontSecret Secret   // Relay: the front server's secret
	Remark      string   // Relay, Forward
	RateMbps    int      // Relay, Forward; RelayManage with action "rate"
	Target      string   // Forward: the friend's node as host:port
	Action      string   // RelayManage: pause, resume, restart, rate or delete
	Items       []string // RelayManage: managed relay IDs
}

// Event tells the window about a task. Step is an executor.Step while a
// deploy or upgrade runs.
type Event struct {
	ID       string   `json:"id"`
	ServerID string   `json:"serverId"`
	FrontID  string   `json:"frontId,omitempty"` // the front server a relay also uses
	Kind     Kind     `json:"kind"`
	State    string   `json:"state"` // running, succeeded, failed or cancelled
	Step     int      `json:"step"`
	Started  int64    `json:"started"`
	Finished int64    `json:"finished,omitempty"`
	Outcome  *Outcome `json:"outcome,omitempty"`
}

// Outcome is a finished task's result.
type Outcome struct {
	Phase      string                       `json:"phase,omitempty"`
	ErrorCode  string                       `json:"errorCode,omitempty"`
	Message    string                       `json:"message,omitempty"`
	NextStep   string                       `json:"nextStep,omitempty"`
	Health     *executor.Health             `json:"health,omitempty"`
	ConfigFile string                       `json:"configFile,omitempty"`
	ConfigPath string                       `json:"configPath,omitempty"`
	Items      []executor.CleanupItem       `json:"items,omitempty"` // what an uninstall removes or removed
	Node       *servers.NodeState           `json:"node,omitempty"`
	Hops       []executor.Hop               `json:"hops,omitempty"`
	Entry      string                       `json:"entry,omitempty"` // a new relay's entry, host:port
	Share      string                       `json:"share,omitempty"` // the text a friend pastes
	Relays     []RelayRow                   `json:"relays,omitempty"`
	Managed    []executor.RelayManageResult `json:"managed,omitempty"`
}

// RelayRow is one managed relay on a server, with what this program knows
// about it.
type RelayRow struct {
	ID          string `json:"id"`
	Role        string `json:"role"` // relay, front or route
	Port        int    `json:"port"`
	Entry       string `json:"entry"` // host:port players connect to
	RateMbps    int    `json:"rateMbps"`
	Target      string `json:"target"`
	State       string `json:"state"` // running, paused or stopped
	Connections int    `json:"connections"`
	CreatedAt   int64  `json:"createdAt"`
	Source      string `json:"source"` // self, friend, front, website, code or unknown
	Remark      string `json:"remark"`
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
	servers   []string // every server the task uses
	cancel    context.CancelFunc
	cancelled atomic.Bool
}

// pendingUninstall holds a preview until the user confirms it. The secret is
// kept so the user need not type it twice for one uninstall.
type pendingUninstall struct {
	serverID  string
	scope     string // msboost or relay
	previewID string
	digest    string
	secret    Secret
	expires   time.Time
}

type session struct {
	secret  Secret
	expires time.Time
}

const confirmWindow = 10 * time.Minute

// sessionWindow is how long relay management on a server reuses the secret
// given when its list was read.
const sessionWindow = 10 * time.Minute

// plan is a validated task, ready to run.
type plan struct {
	kind      Kind
	server    servers.Server
	conn      executor.SSH
	secret    Secret
	front     *servers.Server
	frontConn *executor.SSH
	opts      Options
	config    []byte // Relay: the node configuration
	target    string // Forward
	port      int    // Forward
	confirm   *pendingUninstall
}

// Runner runs tasks. It is safe for concurrent use.
type Runner struct {
	layout   store.Layout
	servers  *servers.Store
	relays   *relays.Store
	log      *logx.Logger
	settings func() store.Settings
	emit     func(Event)

	// Test seams.
	newEngine func(store.Settings) *executor.Engine
	readKey   func(string) ([]byte, error)
	now       func() time.Time

	mu       sync.Mutex
	running  map[string]*job // by server ID
	pending  map[string]pendingUninstall
	sessions map[string]session // by server ID
	records  []Record
}

// New returns a runner. emit receives every event; it must not block. The
// errors are reported problems with the record files; the runner works
// regardless.
func New(layout store.Layout, list *servers.Store, log *logx.Logger, settings func() store.Settings, emit func(Event)) (*Runner, []error) {
	r := &Runner{layout: layout, servers: list, log: log, settings: settings, emit: emit,
		newEngine: defaultEngine, readKey: os.ReadFile, now: time.Now,
		running: map[string]*job{}, pending: map[string]pendingUninstall{}, sessions: map[string]session{}}
	var problems []error
	var err error
	if r.records, err = loadRecords(layout.Tasks); err != nil {
		problems = append(problems, err)
	}
	if r.relays, err = relays.Open(layout.Relays); err != nil {
		problems = append(problems, err)
	}
	return r, problems
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

// SecretNeeded says what to ask before an operation of this kind on the
// server: "password", "passphrase" for an encrypted key, or "" when nothing
// is needed (a key without a passphrase, or relay management within its
// session).
func (r *Runner) SecretNeeded(serverID string, kind Kind) (string, error) {
	sv, ok := r.servers.Get(serverID)
	if !ok {
		return "", errors.New("找不到这台服务器")
	}
	if kind.usesSession() {
		r.mu.Lock()
		_, ok := r.liveSessionLocked(serverID)
		r.mu.Unlock()
		if ok {
			return "", nil
		}
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

// ForgetSession drops the remembered secret for relay management.
func (r *Runner) ForgetSession(serverID string) {
	r.mu.Lock()
	delete(r.sessions, serverID)
	r.mu.Unlock()
}

func (r *Runner) liveSessionLocked(serverID string) (session, bool) {
	s, ok := r.sessions[serverID]
	if ok && r.now().After(s.expires) {
		delete(r.sessions, serverID)
		ok = false
	}
	return s, ok
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

// Start begins an operation. Uninstalls start from a preview and
// ConfirmUninstall.
func (r *Runner) Start(kind Kind, serverID string, secret Secret, opts Options) (Event, error) {
	switch kind {
	case Deploy, Upgrade, Fetch, Status, UninstallPreview, Relay, Forward, RelayList, RelayManage, RelayUninstallPreview:
	default:
		return Event{}, errors.New("不支持的操作")
	}
	sv, ok := r.servers.Get(serverID)
	if !ok {
		return Event{}, errors.New("找不到这台服务器")
	}
	if kind.usesSession() && secret.empty() {
		r.mu.Lock()
		if s, ok := r.liveSessionLocked(serverID); ok {
			secret = s.secret
		}
		r.mu.Unlock()
	}
	p := &plan{kind: kind, server: sv, secret: secret, opts: opts}
	var err error
	if p.conn, err = r.connection(sv, secret); err != nil {
		return Event{}, err
	}
	if err := r.prepare(p); err != nil {
		return Event{}, err
	}
	return r.launch(p)
}

// prepare checks a relay task's options before anything connects.
func (r *Runner) prepare(p *plan) error {
	opts := p.opts
	if len([]rune(opts.Remark)) > 60 || strings.ContainsAny(opts.Remark, "\x00\r\n") {
		return errors.New("备注最多 60 个字，不能换行")
	}
	switch p.kind {
	case Relay, Forward:
		if opts.RateMbps < 0 || opts.RateMbps > executor.MaxRelayRateMbps {
			return errors.New("带宽必须为 1–10 Mbps")
		}
	}
	switch p.kind {
	case Relay:
		raw, err := os.ReadFile(opts.ConfigPath)
		if err != nil {
			return fmt.Errorf("无法读取节点配置：%v", err)
		}
		if _, err := executor.ParseClientConfig(raw); err != nil {
			return fmt.Errorf("节点配置无效：%v", err)
		}
		p.config = raw
		if opts.FrontID != "" {
			front, ok := r.servers.Get(opts.FrontID)
			if !ok || front.ID == p.server.ID {
				return errors.New("前置机必须是另一台服务器")
			}
			conn, err := r.connection(front, opts.FrontSecret)
			if err != nil {
				return fmt.Errorf("前置机：%w", err)
			}
			p.front, p.frontConn = &front, &conn
		}
	case Forward:
		host, port, err := net.SplitHostPort(strings.TrimSpace(opts.Target))
		n, perr := strconv.Atoi(port)
		if err != nil || perr != nil || n < 1 || n > 65535 || host == "" {
			return errors.New("请填写朋友节点的 IP 和端口，例如 203.0.113.10:34917")
		}
		p.target, p.port = host, n
	case RelayManage:
		m := &executor.RelayManage{Action: opts.Action, Items: opts.Items}
		if opts.Action == "rate" {
			m.RateMbps = opts.RateMbps
		}
		if err := executor.ValidateRequest(executor.Request{Kind: "relay-manage", SSH: p.conn, Manage: m}); err != nil {
			return err
		}
	}
	return nil
}

// ConfirmUninstall removes what the preview listed. The server refuses if
// the list changed since.
func (r *Runner) ConfirmUninstall(previewID string) (Event, error) {
	r.mu.Lock()
	pu, ok := r.pending[previewID]
	delete(r.pending, previewID)
	r.sweepLocked()
	r.mu.Unlock()
	if !ok || r.now().After(pu.expires) {
		return Event{}, errors.New("卸载确认已过期，请重新读取卸载清单")
	}
	sv, ok := r.servers.Get(pu.serverID)
	if !ok {
		return Event{}, errors.New("找不到这台服务器")
	}
	conn, err := r.connection(sv, pu.secret)
	if err != nil {
		return Event{}, err
	}
	kind := Uninstall
	if pu.scope == "relay" {
		kind = RelayUninstall
	}
	return r.launch(&plan{kind: kind, server: sv, conn: conn, secret: pu.secret, confirm: &pu})
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
	for id, s := range r.sessions {
		if now.After(s.expires) {
			delete(r.sessions, id)
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
	seen := map[*job]bool{}
	out := make([]Event, 0, len(r.running))
	for _, j := range r.running {
		if !seen[j] {
			seen[j] = true
			out = append(out, j.event)
		}
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

// RelayLabels lists what this program knows about the relays on a server.
func (r *Runner) RelayLabels(serverID string) []relays.Label { return r.relays.ForServer(serverID) }

// ForgetRelays drops the labels of a server removed from the list.
func (r *Runner) ForgetRelays(serverID string) {
	if err := r.relays.RemoveServer(serverID); err != nil {
		r.log.Printf(logx.Task, "无法更新中转记录：%v", err)
	}
	r.ForgetSession(serverID)
}

func (r *Runner) launch(p *plan) (Event, error) {
	ids := []string{p.server.ID}
	if p.front != nil {
		ids = append(ids, p.front.ID)
	}
	r.mu.Lock()
	for _, id := range ids {
		if _, busy := r.running[id]; busy {
			r.mu.Unlock()
			if id != p.server.ID {
				return Event{}, errors.New("前置机上有操作正在进行，请等它结束")
			}
			return Event{}, errors.New("这台服务器上有操作正在进行，请等它结束")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), p.kind.timeout())
	j := &job{event: Event{ID: newID(), ServerID: p.server.ID, Kind: p.kind, State: "running", Started: r.now().UnixMilli()}, servers: ids, cancel: cancel}
	if p.front != nil {
		j.event.FrontID = p.front.ID
	}
	for _, id := range ids {
		r.running[id] = j
	}
	ev := j.event
	r.mu.Unlock()
	r.emit(ev)
	go r.run(ctx, j, p)
	return ev, nil
}

func (r *Runner) run(ctx context.Context, j *job, p *plan) {
	defer j.cancel()
	for _, s := range []string{p.secret.Password, p.secret.Passphrase, p.opts.FrontSecret.Password, p.opts.FrontSecret.Passphrase} {
		if s != "" {
			r.log.AddSecret(s)
			defer r.log.RemoveSecret(s)
		}
	}
	kind, sv := p.kind, p.server
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
	if kind.recorded() {
		r.log.Printf(logx.Task, "开始%s：%s（%s）", kind.Label(), sv.Label(), sv.Address())
	}

	var out Outcome
	var digest string
	state := "failed"
	task := executor.Job{ID: j.event.ID, Script: installers.NodeScript()}
	switch kind {
	case Deploy:
		task.Request = executor.Request{Kind: "deploy", Mode: "fresh", SSH: p.conn}
		state, out = r.finishConfig(sv, e.DeployWithProgress(ctx, task, progress), settings)
	case Upgrade:
		task.Request = executor.Request{SSH: p.conn}
		state, out = r.finishConfig(sv, e.UpgradeWithProgress(ctx, task, progress), settings)
	case Fetch:
		state, out = r.finishConfig(sv, e.FetchConfig(ctx, p.conn), settings)
	case Status:
		st, res := e.NodeStatus(ctx, p.conn)
		state, out = r.finishStatus(sv, st, res)
	case UninstallPreview:
		task.Request = executor.Request{Kind: "cleanup-preview", SSH: p.conn, Cleanup: &executor.CleanupOptions{Scope: "msboost"}}
		state, out, digest = finishPreview(e.Execute(ctx, task), "这台服务器上没有本程序装过的节点组件")
	case Uninstall:
		task.Request = executor.Request{Kind: "cleanup", SSH: p.conn, Cleanup: &executor.CleanupOptions{Scope: "msboost", PreviewID: p.confirm.previewID, Digest: p.confirm.digest, Confirm: true}}
		state, out = r.finishUninstall(sv, e.Execute(ctx, task))
	case Relay:
		task.Request = executor.Request{Kind: "relay", SSH: p.conn, ClientConfig: p.config, Front: p.frontConn, Remark: p.opts.Remark, RateMbps: p.opts.RateMbps}
		state, out = r.finishRelay(p, task.ID, e.Execute(ctx, task), settings)
	case Forward:
		task.Request = executor.Request{SSH: p.conn, RateMbps: p.opts.RateMbps}
		state, out = r.finishForward(p, task.ID, e.ForwardRelay(ctx, task, p.target, p.port))
	case RelayList:
		task.Request = executor.Request{Kind: "relay-inventory", SSH: p.conn}
		state, out = r.finishList(sv, e.Execute(ctx, task))
	case RelayManage:
		m := &executor.RelayManage{Action: p.opts.Action, Items: p.opts.Items}
		if m.Action == "rate" {
			m.RateMbps = p.opts.RateMbps
		}
		task.Request = executor.Request{Kind: "relay-manage", SSH: p.conn, Manage: m}
		state, out = r.finishManage(sv, m, e.Execute(ctx, task))
	case RelayUninstallPreview:
		task.Request = executor.Request{Kind: "cleanup-preview", SSH: p.conn, Cleanup: &executor.CleanupOptions{Scope: "relay"}}
		state, out, digest = finishPreview(e.Execute(ctx, task), "这台服务器上没有中转")
	case RelayUninstall:
		task.Request = executor.Request{Kind: "cleanup", SSH: p.conn, Cleanup: &executor.CleanupOptions{Scope: "relay", PreviewID: p.confirm.previewID, Digest: p.confirm.digest, Confirm: true}}
		state, out = r.finishRelayUninstall(sv, e.Execute(ctx, task))
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
	for _, id := range j.servers {
		if r.running[id] == j {
			delete(r.running, id)
		}
	}
	if digest != "" {
		scope := "msboost"
		if kind == RelayUninstallPreview {
			scope = "relay"
		}
		r.pending[ev.ID] = pendingUninstall{serverID: sv.ID, scope: scope, previewID: ev.ID, digest: digest, secret: p.secret, expires: r.now().Add(confirmWindow)}
	}
	if kind.usesSession() && state == "succeeded" {
		r.sessions[sv.ID] = session{secret: p.secret, expires: r.now().Add(sessionWindow)}
	}
	if kind.recorded() {
		r.records = append([]Record{{ID: ev.ID, ServerID: sv.ID, Server: sv.Label(), Address: sv.Address(), Fingerprint: sv.Fingerprint,
			Kind: kind, State: state, Started: ev.Started, Finished: ev.Finished, Outcome: recordOutcome(out)}}, r.records...)
		if len(r.records) > maxRecords {
			r.records = r.records[:maxRecords]
		}
		if err := store.WriteJSON(r.layout.Tasks, recordsFile{Version: 1, Records: r.records}); err != nil {
			r.log.Printf(logx.Task, "无法保存任务记录：%v", err)
		}
	}
	r.mu.Unlock()
	if kind.recorded() {
		if state == "succeeded" {
			r.log.Printf(logx.Task, "%s完成：%s", kind.Label(), sv.Label())
		} else {
			r.log.Printf(logx.Task, "%s未完成：%s，%s", kind.Label(), sv.Label(), out.Message)
		}
	}
	r.emit(ev)
}

// recordOutcome drops the bulky lists a record does not need.
func recordOutcome(o Outcome) Outcome {
	o.Relays = nil
	return o
}

// failure turns an engine result into an outcome.
func failure(res executor.Result) Outcome {
	return Outcome{Phase: res.Phase, ErrorCode: res.ErrorCode, Message: res.Message, NextStep: res.NextStep, Health: res.Health}
}

// saveConfig writes a configuration where the launcher reads it. own is the
// file this configuration replaces, if any.
func (r *Runner) saveConfig(base, own string, data []byte, settings store.Settings) (dir, name, path string, err error) {
	dir = r.layout.ConfigsDir(settings)
	name = store.FreeConfigName(dir, store.ConfigName(base), own)
	path, err = store.SaveConfig(dir, name, data)
	return dir, name, path, err
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
	own := ""
	if sv.ConfigDir == r.layout.ConfigsDir(settings) {
		own = sv.ConfigFile
	}
	dir, name, path, err := r.saveConfig(sv.Label(), own, res.Config, settings)
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
	r.updateServer(sv.ID, func(v *servers.Server) { v.ConfigFile, v.ConfigDir, v.Node = name, dir, node })
	return "succeeded", Outcome{Phase: res.Phase, Message: res.Message, Health: res.Health, ConfigFile: name, ConfigPath: path, Node: node}
}

func (r *Runner) finishStatus(sv servers.Server, st executor.NodeStatus, res executor.Result) (string, Outcome) {
	if res.State != "succeeded" {
		return "failed", failure(res)
	}
	now := r.now().UnixMilli()
	node := &servers.NodeState{Installed: st.Installed, Service: st.Service, Engine: st.Engine, BBR: st.BBR, OS: st.OS,
		Entry: st.Entry, PublicTCP: st.PublicTCP, CheckedAt: now}
	r.updateServer(sv.ID, func(v *servers.Server) {
		v.Node = node
		v.Relays = &servers.RelaySummary{Count: st.Relays, CheckedAt: now}
	})
	return "succeeded", Outcome{Node: node}
}

// finishPreview also returns the preview's digest, which the confirmation
// must present; it is empty when there is nothing to remove.
func finishPreview(res executor.Result, nothing string) (string, Outcome, string) {
	if res.State != "succeeded" || res.Cleanup == nil {
		return "failed", failure(res), ""
	}
	if len(res.Cleanup.Items) == 0 {
		return "succeeded", Outcome{Message: nothing}, ""
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
	r.updateServer(sv.ID, func(v *servers.Server) { v.ConfigFile, v.ConfigDir, v.Node = "", "", node })
	out.Node = node
	return "succeeded", out
}

// finishRelay saves the configuration with the relay's entry and records
// what the new relay (and its front) are for.
func (r *Runner) finishRelay(p *plan, taskID string, res executor.Result, settings store.Settings) (string, Outcome) {
	if res.State != "succeeded" || len(res.Hops) == 0 {
		return "failed", failure(res)
	}
	base := p.opts.Remark
	if base == "" {
		base = "自备中转"
	}
	_, name, path, err := r.saveConfig(base, "", res.Config, settings)
	if err != nil {
		return "failed", Outcome{Phase: "config", ErrorCode: "config_not_saved", Message: err.Error(),
			NextStep: "中转已经建好，但配置没能保存。腾出磁盘空间后删除这条中转并重新创建。"}
	}
	hop := func(h executor.Hop) (string, string) {
		return net.JoinHostPort(h.FromHost, strconv.Itoa(h.FromPort)), net.JoinHostPort(h.ToHost, strconv.Itoa(h.ToPort))
	}
	now := r.now().UnixMilli()
	relayEntry, nodeEntry := hop(res.Hops[0])
	labels := []relays.Label{{ID: taskID + "r", ServerID: p.server.ID, Kind: relays.Self, Remark: p.opts.Remark, Target: nodeEntry, Entry: relayEntry, ConfigFile: name, CreatedAt: now}}
	entry := relayEntry
	if p.front != nil && len(res.Hops) > 1 {
		frontEntry, _ := hop(res.Hops[1])
		labels = append(labels, relays.Label{ID: taskID + "f", ServerID: p.front.ID, Kind: relays.Front, Remark: p.opts.Remark, Target: relayEntry, Entry: frontEntry, ConfigFile: name, CreatedAt: now})
		entry = frontEntry
		r.bumpRelays(p.front.ID, 1)
	}
	r.bumpRelays(p.server.ID, 1)
	if err := r.relays.Add(labels...); err != nil {
		r.log.Printf(logx.Task, "无法保存中转记录：%v", err)
	}
	return "succeeded", Outcome{Phase: res.Phase, Message: res.Message, Health: res.Health, ConfigFile: name, ConfigPath: path, Hops: res.Hops, Entry: entry}
}

// finishForward records a friend's relay and the text to send back.
func (r *Runner) finishForward(p *plan, taskID string, res executor.Result) (string, Outcome) {
	if res.State != "succeeded" || len(res.Hops) == 0 {
		return "failed", failure(res)
	}
	h := res.Hops[0]
	entry := net.JoinHostPort(h.FromHost, strconv.Itoa(h.FromPort))
	target := net.JoinHostPort(h.ToHost, strconv.Itoa(h.ToPort))
	if err := r.relays.Add(relays.Label{ID: taskID + "r", ServerID: p.server.ID, Kind: relays.Friend, Remark: p.opts.Remark,
		Target: target, Entry: entry, CreatedAt: r.now().UnixMilli()}); err != nil {
		r.log.Printf(logx.Task, "无法保存中转记录：%v", err)
	}
	r.bumpRelays(p.server.ID, 1)
	return "succeeded", Outcome{Phase: res.Phase, Message: res.Message, Health: res.Health, Hops: res.Hops, Entry: entry, Share: ShareEntry(entry, target)}
}

func (r *Runner) finishList(sv servers.Server, res executor.Result) (string, Outcome) {
	if res.State != "succeeded" {
		return "failed", failure(res)
	}
	rows := make([]RelayRow, 0, len(res.Inventory))
	for _, e := range res.Inventory {
		row := RelayRow{ID: e.ID, Role: e.Role, Port: e.Port, Entry: net.JoinHostPort(sv.Host, strconv.Itoa(e.Port)), RateMbps: e.RateMbps,
			Target: e.Target, State: e.State, Connections: e.Connections, CreatedAt: e.CreatedAt, Source: "unknown"}
		if l, ok := r.relays.Find(sv.ID, e.ID); ok {
			row.Source, row.Remark = l.Kind, l.Remark
		} else if rec := e.Record; rec != nil {
			row.Source = "website"
			if rec.Source == "code" {
				row.Source = "code"
			}
			row.Remark = maskEmail(rec.User)
			if rec.Remark != "" {
				row.Remark += " · " + rec.Remark
			}
		}
		rows = append(rows, row)
	}
	r.updateServer(sv.ID, func(v *servers.Server) {
		v.Relays = &servers.RelaySummary{Count: len(rows), CheckedAt: r.now().UnixMilli()}
	})
	return "succeeded", Outcome{Message: res.Message, Relays: rows}
}

func (r *Runner) finishManage(sv servers.Server, m *executor.RelayManage, res executor.Result) (string, Outcome) {
	if res.State != "succeeded" {
		return "failed", failure(res)
	}
	out := Outcome{Managed: res.Managed}
	var deleted []string
	failedCount := 0
	for _, item := range res.Managed {
		if !item.OK {
			failedCount++
		} else if m.Action == "delete" {
			deleted = append(deleted, item.ID)
		}
	}
	if len(deleted) > 0 {
		if err := r.relays.Remove(sv.ID, deleted...); err != nil {
			r.log.Printf(logx.Task, "无法更新中转记录：%v", err)
		}
		r.bumpRelays(sv.ID, -len(deleted))
	}
	if failedCount > 0 {
		out.Message = fmt.Sprintf("%d 条中转没有处理成功", failedCount)
		return "failed", out
	}
	out.Message = map[string]string{"pause": "已暂停", "resume": "已恢复", "restart": "已重启", "rate": "已修改带宽", "delete": "已删除"}[m.Action]
	return "succeeded", out
}

func (r *Runner) finishRelayUninstall(sv servers.Server, res executor.Result) (string, Outcome) {
	if res.State != "succeeded" || res.Cleanup == nil {
		return "failed", failure(res)
	}
	if err := r.relays.RemoveServer(sv.ID); err != nil {
		r.log.Printf(logx.Task, "无法更新中转记录：%v", err)
	}
	r.updateServer(sv.ID, func(v *servers.Server) {
		v.Relays = &servers.RelaySummary{CheckedAt: r.now().UnixMilli()}
	})
	return "succeeded", Outcome{Items: res.Cleanup.Items, Message: "这台服务器上的中转已全部卸载，经过它们的配置都已失效"}
}

// bumpRelays adjusts a known relay count after a relay is added or deleted.
func (r *Runner) bumpRelays(serverID string, delta int) {
	r.updateServer(serverID, func(v *servers.Server) {
		if v.Relays != nil {
			v.Relays.Count = max(0, v.Relays.Count+delta)
		}
	})
}

func (r *Runner) updateServer(id string, change func(*servers.Server)) {
	if _, err := r.servers.Update(id, func(v *servers.Server) error {
		change(v)
		return nil
	}); err != nil {
		r.log.Printf(logx.Task, "无法更新服务器记录：%v", err)
	}
}

// maskEmail keeps a website user's e-mail recognisable without spelling it out.
func maskEmail(email string) string {
	at := strings.LastIndex(email, "@")
	if at < 1 {
		return ""
	}
	return email[:1] + "***" + email[at:]
}

func newID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
