package tasks

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/mozziexwz/node_local/internal/executor"
	"github.com/mozziexwz/node_local/internal/logx"
	"github.com/mozziexwz/node_local/internal/relays"
	"github.com/mozziexwz/node_local/internal/servers"
	"github.com/mozziexwz/node_local/internal/store"
)

const (
	sshPassword = "ssh-pass-123"
	nodeConfig  = `{"profiles":[{"profileName":"msboost","user":{"name":"msboost-a1","password":"node-secret"},"servers":[{"ipAddress":"8.8.8.8","domainName":"","portBindings":[{"port":30000,"protocol":"TCP"}]}],"mtu":1400}],"activeProfile":"msboost","rpcPort":8964,"socks5Port":10086}`
	digest      = "3a9e812718ccac89f62700b2daedc0ddb1dfeebd5a9f029c6f6bf9d686d7a7fa"
)

// relayPorts are the entry ports the fake relay servers bind.
var relayPorts = map[string]string{"1.1.1.1": "41203", "8.8.4.4": "52880"}

var manageItems = regexp.MustCompile(`manage_items=\$\(printf '%s' '([A-Za-z0-9+/=]*)'`)

// fakeServer answers the engine's scripts like a Debian server would. With
// block set, every script waits until the task is cancelled.
type fakeServer struct {
	mu        sync.Mutex
	scripts   []string
	hosts     []string
	block     bool
	fail      error
	inventory string // JSON of the relay list
}

func (f *fakeServer) Probe(context.Context, string, int) (string, string, error) {
	return "SHA256:" + strings.Repeat("A", 43), "ssh-ed25519", nil
}

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func cleanupReport(scope string, removed bool) string {
	items := `[{"path":"/etc/msboost","kind":"directory"},{"path":"/root/直连.json","kind":"file"}]`
	if scope == "relay" {
		items = `[{"path":"/etc/msboost-free/oldr","kind":"directory"},{"path":"/etc/systemd/system/msboost-free-oldr.service","kind":"file"}]`
	}
	report := `{"scope":"` + scope + `","digest":"` + digest + `","items":` + items + `,"removed":` + map[bool]string{true: "true", false: "false"}[removed] + `}`
	return "MSBOOST_CLEANUP=" + b64(report) + "\n"
}

func (f *fakeServer) Run(ctx context.Context, s executor.SSH, script string) ([]byte, error) {
	f.mu.Lock()
	f.scripts = append(f.scripts, script)
	f.hosts = append(f.hosts, s.Host)
	f.mu.Unlock()
	if f.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if f.fail != nil {
		return nil, f.fail
	}
	switch {
	case script == "uname -m\n":
		return []byte("x86_64\n"), nil
	case strings.Contains(script, "MSBOOST_READY=1"):
		return []byte("MSBOOST_READY=1\n"), nil
	case strings.Contains(script, "MSBOOST_FORCE_FRESH=1"):
		return []byte("MSBOOST_BBR=enabled\nMSBOOST_STEP=Installing required packages\nMSBOOST_CONFIG=" + b64(nodeConfig) + "\n"), nil
	case strings.Contains(script, "MSBOOST_RELAY_PORT"):
		port := relayPorts[s.Host]
		if port == "" {
			port = "33333"
		}
		return []byte("MSBOOST_BBR=enabled\nMSBOOST_RELAY_PORT=" + port + "\n"), nil
	case strings.Contains(script, "MSBOOST_INVENTORY"):
		return []byte("MSBOOST_OS=Debian 12\nMSBOOST_INVENTORY=" + b64(f.inventory) + "\n"), nil
	case strings.Contains(script, "MSBOOST_MANAGE"):
		m := manageItems.FindStringSubmatch(script)
		raw, _ := base64.StdEncoding.DecodeString(m[1])
		var results []string
		for _, id := range strings.Fields(string(raw)) {
			results = append(results, `{"id":"`+id+`","ok":true}`)
		}
		return []byte("MSBOOST_MANAGE=" + b64("["+strings.Join(results, ",")+"]") + "\n"), nil
	case strings.Contains(script, "MSBOOST_INSTALLED"):
		return []byte("MSBOOST_OS=debian 12\nMSBOOST_BBR=bbr\nMSBOOST_RELAYS=2\nMSBOOST_INSTALLED=1\nMSBOOST_SERVICE=active\nMSBOOST_PROBE=active\nMSBOOST_ENGINE=v1.19.32\nMSBOOST_CONFIG=" + b64(nodeConfig) + "\n"), nil
	case strings.Contains(script, "not_installed"):
		return []byte("MSBOOST_SERVICE=active\nMSBOOST_PROBE=active\nMSBOOST_CONFIG=" + b64(nodeConfig) + "\n"), nil
	case strings.Contains(script, "MSBOOST_CLEANUP_PY"):
		scope := "msboost"
		if strings.Contains(script, b64("relay")) {
			scope = "relay"
		}
		return []byte(cleanupReport(scope, !strings.Contains(script, b64("cleanup-preview")))), nil
	}
	return nil, errors.New("unexpected script")
}

type harness struct {
	t       *testing.T
	runner  *Runner
	layout  store.Layout
	list    *servers.Store
	server  servers.Server // the node, 8.8.8.8
	relay   servers.Server // a relay server, 1.1.1.1
	front   servers.Server // a front server, 8.8.4.4
	fake    *fakeServer
	log     *logx.Logger
	logDir  string
	events  chan Event
	setting store.Settings
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{t: t, fake: &fakeServer{}, events: make(chan Event, 100), setting: store.DefaultSettings()}
	h.layout = store.NewLayout(t.TempDir())
	if err := h.layout.Ensure(); err != nil {
		t.Fatal(err)
	}
	h.logDir = filepath.Join(t.TempDir(), "logs")
	var err error
	if h.log, err = logx.New(h.logDir, 7); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.log.Close() })
	h.list, _ = servers.Open(h.layout.Servers)
	add := func(name, host string) servers.Server {
		sv, err := h.list.Add(servers.Server{Name: name, Host: host, Port: 22, AuthMethod: "password", Fingerprint: "SHA256:" + strings.Repeat("A", 43)})
		if err != nil {
			t.Fatal(err)
		}
		return sv
	}
	h.server, h.relay, h.front = add("东京节点", "8.8.8.8"), add("洛杉矶中转", "1.1.1.1"), add("前置", "8.8.4.4")
	var problems []error
	h.runner, problems = New(h.layout, h.list, h.log, func() store.Settings { return h.setting }, func(e Event) { h.events <- e })
	if len(problems) > 0 {
		t.Fatal(problems)
	}
	h.runner.newEngine = func(store.Settings) *executor.Engine {
		return &executor.Engine{Remote: h.fake, TCPProbe: func(context.Context, string, int) string { return "reachable" },
			GostAMD64URL: "https://github.com/go-gost/gost/releases/download/v3.3.0/gost.tar.gz", GostAMD64SHA256: strings.Repeat("a", 64)}
	}
	return h
}

func (h *harness) start(kind Kind, sv servers.Server, opts Options) Event {
	h.t.Helper()
	ev, err := h.runner.Start(kind, sv.ID, Secret{Password: sshPassword}, opts)
	if err != nil {
		h.t.Fatal(err)
	}
	return ev
}

// wait returns the task's final event and every step seen on the way.
func (h *harness) wait(id string) (Event, []int) {
	h.t.Helper()
	var steps []int
	for {
		select {
		case e := <-h.events:
			if e.ID != id {
				continue
			}
			if e.State != "running" {
				return e, steps
			}
			if e.Step > 0 {
				steps = append(steps, e.Step)
			}
		case <-time.After(10 * time.Second):
			h.t.Fatal("task did not finish")
		}
	}
}

func TestDeploySavesTheConfigurationAndARecordWithoutSecrets(t *testing.T) {
	h := newHarness(t)
	final, steps := h.wait(h.start(Deploy, h.server, Options{}).ID)
	if final.State != "succeeded" || final.Outcome.ConfigFile != "东京节点.json" || final.Outcome.Health.PublicTCP != "reachable" {
		t.Fatalf("final %+v %+v", final, final.Outcome)
	}
	if len(steps) == 0 || steps[len(steps)-1] != int(executor.StepConfig) {
		t.Fatalf("steps %v", steps)
	}
	saved, err := os.ReadFile(filepath.Join(h.layout.Configs, "东京节点.json"))
	if err != nil || string(saved) != nodeConfig {
		t.Fatalf("saved config %q %v", saved, err)
	}
	sv, _ := h.list.Get(h.server.ID)
	if sv.ConfigFile != "东京节点.json" || sv.Node == nil || !sv.Node.Installed || sv.Node.Entry != "8.8.8.8:30000" || sv.Node.BBR != "enabled" {
		t.Fatalf("server %+v node %+v", sv, sv.Node)
	}
	records := h.runner.Records()
	if len(records) != 1 || records[0].Kind != Deploy || records[0].Server != "东京节点" {
		t.Fatalf("records %+v", records)
	}
	h.log.Close()
	for _, path := range []string{h.layout.Tasks, h.layout.Servers} {
		raw, _ := os.ReadFile(path)
		if strings.Contains(string(raw), sshPassword) || strings.Contains(string(raw), "node-secret") {
			t.Fatalf("%s holds a secret", path)
		}
	}
	logs, _ := filepath.Glob(filepath.Join(h.logDir, "*.log"))
	for _, path := range logs {
		raw, _ := os.ReadFile(path)
		if strings.Contains(string(raw), sshPassword) {
			t.Fatal("the log holds the SSH password")
		}
	}

	// A second deploy replaces the server's own file instead of adding one.
	if final, _ = h.wait(h.start(Deploy, h.server, Options{}).ID); final.Outcome.ConfigFile != "东京节点.json" {
		t.Fatalf("redeploy wrote %s", final.Outcome.ConfigFile)
	}
}

func TestOneTaskPerServerAndCancel(t *testing.T) {
	h := newHarness(t)
	h.fake.block = true
	ev := h.start(Deploy, h.server, Options{})
	if _, err := h.runner.Start(Fetch, h.server.ID, Secret{Password: sshPassword}, Options{}); err == nil {
		t.Fatal("a second task on the same server was accepted")
	}
	if !h.runner.Busy() {
		t.Fatal("runner not busy")
	}
	h.runner.Cancel(ev.ID)
	final, _ := h.wait(ev.ID)
	if final.State != "cancelled" || !strings.Contains(final.Outcome.NextStep, "取回配置") {
		t.Fatalf("cancelled task %+v %+v", final, final.Outcome)
	}
	if h.runner.Busy() {
		t.Fatal("still busy after cancelling")
	}
}

func TestUninstallRequiresThePreview(t *testing.T) {
	h := newHarness(t)
	h.wait(h.start(Deploy, h.server, Options{}).ID)

	preview := h.start(UninstallPreview, h.server, Options{})
	final, _ := h.wait(preview.ID)
	if final.State != "succeeded" || len(final.Outcome.Items) != 2 {
		t.Fatalf("preview %+v %+v", final, final.Outcome)
	}
	if _, err := h.runner.ConfirmUninstall("unknown"); err == nil {
		t.Fatal("confirmed an unknown preview")
	}
	run, err := h.runner.ConfirmUninstall(preview.ID)
	if err != nil {
		t.Fatal(err)
	}
	final, _ = h.wait(run.ID)
	if final.State != "succeeded" || final.Kind != Uninstall || !strings.Contains(final.Outcome.Message, "东京节点.json 已经失效") {
		t.Fatalf("uninstall %+v %+v", final, final.Outcome)
	}
	last := h.fake.scripts[len(h.fake.scripts)-1]
	if !strings.Contains(last, b64(digest)) {
		t.Fatal("the uninstall did not present the preview's digest")
	}
	sv, _ := h.list.Get(h.server.ID)
	if sv.ConfigFile != "" || sv.Node == nil || sv.Node.Installed {
		t.Fatalf("server after uninstall %+v", sv)
	}
	if _, err := h.runner.ConfirmUninstall(preview.ID); err == nil {
		t.Fatal("a preview was confirmed twice")
	}
}

func TestFailuresUseLocalWording(t *testing.T) {
	h := newHarness(t)
	h.fake.fail = executor.PublicDiagnostic("ssh_timeout", "")
	final, _ := h.wait(h.start(Fetch, h.server, Options{}).ID)
	if final.State != "failed" || final.Outcome.ErrorCode != "ssh_timeout" || strings.Contains(final.Outcome.NextStep, "执行机") || !strings.Contains(final.Outcome.NextStep, "SSH 代理") {
		t.Fatalf("outcome %+v", final.Outcome)
	}
	h.setting.Proxy.Enabled = true
	if final, _ = h.wait(h.start(Fetch, h.server, Options{}).ID); final.Outcome.NextStep != proxyHint {
		t.Fatalf("with the proxy on: %+v", final.Outcome)
	}
	if records := h.runner.Records(); len(records) != 2 || records[0].State != "failed" {
		t.Fatalf("records %+v", records)
	}
}

func TestConfigurationGoesToTheLauncher(t *testing.T) {
	h := newHarness(t)
	launcher := t.TempDir()
	os.WriteFile(filepath.Join(launcher, store.LauncherExe), nil, 0o644)
	h.setting.LauncherDir = launcher
	final, _ := h.wait(h.start(Fetch, h.server, Options{}).ID)
	if final.Outcome.ConfigPath != filepath.Join(launcher, "configs", "东京节点.json") {
		t.Fatalf("saved to %s", final.Outcome.ConfigPath)
	}
}

func TestStatusUpdatesTheServer(t *testing.T) {
	h := newHarness(t)
	final, _ := h.wait(h.start(Status, h.server, Options{}).ID)
	if final.State != "succeeded" || final.Outcome.Node == nil || final.Outcome.Node.Engine != "v1.19.32" || final.Outcome.Node.OS != "Debian 12" {
		t.Fatalf("status %+v", final.Outcome)
	}
	if sv, _ := h.list.Get(h.server.ID); sv.Relays == nil || sv.Relays.Count != 2 {
		t.Fatalf("relay count %+v", sv.Relays)
	}
	if len(h.runner.Records()) != 0 {
		t.Fatal("status checks are not recorded")
	}
}

func TestSecretNeeded(t *testing.T) {
	h := newHarness(t)
	if got, err := h.runner.SecretNeeded(h.server.ID, Deploy); got != "password" || err != nil {
		t.Fatalf("password server: %q %v", got, err)
	}
	_, key, _ := ed25519.GenerateKey(rand.Reader)
	plain, _ := ssh.MarshalPrivateKey(key, "")
	locked, _ := ssh.MarshalPrivateKeyWithPassphrase(key, "", []byte("phrase"))
	keys := map[string][]byte{"plain": pem.EncodeToMemory(plain), "locked": pem.EncodeToMemory(locked), "junk": []byte("not a key")}
	h.runner.readKey = func(path string) ([]byte, error) { return keys[filepath.Base(path)], nil }
	for name, want := range map[string]string{"plain": "", "locked": "passphrase"} {
		sv, err := h.list.Update(h.server.ID, func(v *servers.Server) error {
			v.AuthMethod, v.KeyPath = "key", filepath.Join(t.TempDir(), name)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if got, err := h.runner.SecretNeeded(sv.ID, Deploy); got != want || err != nil {
			t.Errorf("%s key: %q %v", name, got, err)
		}
	}
	h.list.Update(h.server.ID, func(v *servers.Server) error { v.KeyPath = filepath.Join(t.TempDir(), "junk"); return nil })
	if _, err := h.runner.SecretNeeded(h.server.ID, Deploy); err == nil {
		t.Fatal("accepted a broken key file")
	}
	if _, err := h.runner.Start(Fetch, h.server.ID, Secret{}, Options{}); err == nil {
		t.Fatal("started with an unreadable key")
	}
}

// writeNodeConfig puts the node's configuration where the launcher reads it.
func writeNodeConfig(t *testing.T, h *harness) string {
	t.Helper()
	path, err := store.SaveConfig(h.layout.Configs, "东京节点.json", []byte(nodeConfig))
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSelfRelayWithFront(t *testing.T) {
	h := newHarness(t)
	path := writeNodeConfig(t, h)
	opts := Options{ConfigPath: path, FrontID: h.front.ID, FrontSecret: Secret{Password: "front-pass-99"}, Remark: "经洛杉矶", RateMbps: 8}
	ev, err := h.runner.Start(Relay, h.relay.ID, Secret{Password: sshPassword}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if ev.FrontID != h.front.ID {
		t.Fatalf("event %+v", ev)
	}
	if _, err := h.runner.Start(Status, h.front.ID, Secret{Password: sshPassword}, Options{}); err == nil {
		// The fake answers at once, so the front may already be free; only a
		// busy refusal is checked here.
	} else if !strings.Contains(err.Error(), "正在进行") {
		t.Fatalf("front lock: %v", err)
	}
	final, _ := h.wait(ev.ID)
	if final.State != "succeeded" || final.Outcome.Entry != "8.8.4.4:52880" || len(final.Outcome.Hops) != 2 || final.Outcome.ConfigFile != "经洛杉矶.json" {
		t.Fatalf("relay %+v %+v", final, final.Outcome)
	}
	saved, _ := os.ReadFile(final.Outcome.ConfigPath)
	info, err := executor.ParseClientConfig(saved)
	if err != nil || info.TargetHost != "8.8.4.4" || info.TargetPort != 52880 || !strings.Contains(string(saved), `"profileName": "经洛杉矶"`) {
		t.Fatalf("saved %s %v", saved, err)
	}
	labels := h.runner.RelayLabels(h.relay.ID)
	if len(labels) != 1 || labels[0].Kind != relays.Self || labels[0].Target != "8.8.8.8:30000" || labels[0].Entry != "1.1.1.1:41203" {
		t.Fatalf("relay labels %+v", labels)
	}
	if fl := h.runner.RelayLabels(h.front.ID); len(fl) != 1 || fl[0].Kind != relays.Front || fl[0].Target != "1.1.1.1:41203" {
		t.Fatalf("front labels %+v", fl)
	}
	for _, host := range h.fake.hosts {
		if host != "1.1.1.1" && host != "8.8.4.4" {
			t.Fatalf("the relay task connected to %s", host)
		}
	}
	h.log.Close()
	logs, _ := filepath.Glob(filepath.Join(h.logDir, "*.log"))
	for _, p := range logs {
		raw, _ := os.ReadFile(p)
		if strings.Contains(string(raw), "front-pass-99") {
			t.Fatal("the log holds the front's password")
		}
	}
}

func TestRelayRejectsBadOptions(t *testing.T) {
	h := newHarness(t)
	path := writeNodeConfig(t, h)
	for name, opts := range map[string]Options{
		"missing config":   {ConfigPath: filepath.Join(t.TempDir(), "none.json")},
		"front is relay":   {ConfigPath: path, FrontID: h.relay.ID, FrontSecret: Secret{Password: "x"}},
		"front no secret":  {ConfigPath: path, FrontID: h.front.ID},
		"rate too high":    {ConfigPath: path, RateMbps: 11},
		"remark multiline": {ConfigPath: path, Remark: "a\nb"},
	} {
		if _, err := h.runner.Start(Relay, h.relay.ID, Secret{Password: sshPassword}, opts); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := h.runner.Start(Forward, h.relay.ID, Secret{Password: sshPassword}, Options{Target: "not an address"}); err == nil {
		t.Error("forward without an address was accepted")
	}
	if _, err := h.runner.Start(RelayManage, h.relay.ID, Secret{Password: sshPassword}, Options{Action: "reboot", Items: []string{"ar"}}); err == nil {
		t.Error("an unknown relay action was accepted")
	}
}

func TestFriendRelayListManageAndUninstall(t *testing.T) {
	h := newHarness(t)
	forward, _ := h.wait(h.start(Forward, h.relay, Options{Target: "9.9.9.9:30512", Remark: "小明", RateMbps: 5}).ID)
	if forward.State != "succeeded" || forward.Outcome.Entry != "1.1.1.1:41203" || forward.Outcome.Share != "MSBOOST 中转入口 1.1.1.1:41203 → 9.9.9.9:30512" {
		t.Fatalf("forward %+v %+v", forward, forward.Outcome)
	}
	id := forward.ID + "r"

	h.fake.inventory = `[{"id":"` + id + `","role":"relay","port":41203,"rateMbps":5,"target":"9.9.9.9:30512","state":"running","connections":1,"createdAt":1},` +
		`{"id":"webr","role":"relay","port":50000,"rateMbps":10,"target":"8.8.8.8:30000","state":"paused","connections":0,"createdAt":2,` +
		`"record":{"version":1,"source":"manual","user":"abc@qq.com","target":"8.8.8.8:30000","port":50000,"rateMbps":10,"remark":"网站建的","createdAt":"2026-10-20T00:00:00Z"}}]`
	list, _ := h.wait(h.start(RelayList, h.relay, Options{}).ID)
	rows := list.Outcome.Relays
	if list.State != "succeeded" || len(rows) != 2 {
		t.Fatalf("list %+v", list.Outcome)
	}
	if rows[0].Source != relays.Friend || rows[0].Remark != "小明" || rows[0].Entry != "1.1.1.1:41203" {
		t.Fatalf("friend row %+v", rows[0])
	}
	if rows[1].Source != "website" || rows[1].Remark != "a***@qq.com · 网站建的" {
		t.Fatalf("website row %+v", rows[1])
	}
	if sv, _ := h.list.Get(h.relay.ID); sv.Relays == nil || sv.Relays.Count != 2 {
		t.Fatalf("relay count %+v", sv.Relays)
	}

	// Within the session, managing needs no password.
	if need, _ := h.runner.SecretNeeded(h.relay.ID, RelayManage); need != "" {
		t.Fatalf("session not used: %q", need)
	}
	ev, err := h.runner.Start(RelayManage, h.relay.ID, Secret{}, Options{Action: "delete", Items: []string{id}})
	if err != nil {
		t.Fatal(err)
	}
	managed, _ := h.wait(ev.ID)
	if managed.State != "succeeded" || len(managed.Outcome.Managed) != 1 || !managed.Outcome.Managed[0].OK {
		t.Fatalf("manage %+v", managed.Outcome)
	}
	if _, ok := h.runner.relays.Find(h.relay.ID, id); ok {
		t.Fatal("the deleted relay's label remained")
	}
	if sv, _ := h.list.Get(h.relay.ID); sv.Relays.Count != 1 {
		t.Fatalf("relay count after delete %+v", sv.Relays)
	}
	h.runner.ForgetSession(h.relay.ID)
	if need, _ := h.runner.SecretNeeded(h.relay.ID, RelayManage); need != "password" {
		t.Fatalf("forgotten session still used: %q", need)
	}

	preview := h.start(RelayUninstallPreview, h.relay, Options{})
	p, _ := h.wait(preview.ID)
	if p.State != "succeeded" || len(p.Outcome.Items) != 2 {
		t.Fatalf("relay preview %+v", p.Outcome)
	}
	run, err := h.runner.ConfirmUninstall(preview.ID)
	if err != nil {
		t.Fatal(err)
	}
	done, _ := h.wait(run.ID)
	if done.State != "succeeded" || done.Kind != RelayUninstall {
		t.Fatalf("relay uninstall %+v %+v", done, done.Outcome)
	}
	if sv, _ := h.list.Get(h.relay.ID); sv.Relays == nil || sv.Relays.Count != 0 {
		t.Fatalf("relay count after uninstall %+v", sv.Relays)
	}
	kinds := map[Kind]bool{}
	for _, r := range h.runner.Records() {
		kinds[r.Kind] = true
		if len(r.Outcome.Relays) != 0 {
			t.Fatal("records must not keep relay lists")
		}
	}
	if !kinds[Forward] || !kinds[RelayManage] || !kinds[RelayUninstall] || kinds[RelayList] {
		t.Fatalf("recorded kinds %v", kinds)
	}
}

func TestForwardRefusesItself(t *testing.T) {
	h := newHarness(t)
	final, _ := h.wait(h.start(Forward, h.relay, Options{Target: "1.1.1.1:30512"}).ID)
	if final.State != "failed" || final.Outcome.ErrorCode != "invalid_target" {
		t.Fatalf("forward to itself %+v", final.Outcome)
	}
}
