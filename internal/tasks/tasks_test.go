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
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/mozziexwz/node_local/internal/executor"
	"github.com/mozziexwz/node_local/internal/logx"
	"github.com/mozziexwz/node_local/internal/servers"
	"github.com/mozziexwz/node_local/internal/store"
)

const (
	sshPassword = "ssh-pass-123"
	nodeConfig  = `{"profiles":[{"profileName":"msboost","user":{"name":"msboost-a1","password":"node-secret"},"servers":[{"ipAddress":"8.8.8.8","domainName":"","portBindings":[{"port":30000,"protocol":"TCP"}]}],"mtu":1400}],"activeProfile":"msboost","rpcPort":8964,"socks5Port":10086}`
	digest      = "3a9e812718ccac89f62700b2daedc0ddb1dfeebd5a9f029c6f6bf9d686d7a7fa"
)

// fakeServer answers the engine's scripts like a Debian server would. With
// block set, every script waits until the task is cancelled.
type fakeServer struct {
	mu      sync.Mutex
	scripts []string
	block   bool
	fail    error
}

func (f *fakeServer) Probe(context.Context, string, int) (string, string, error) {
	return "SHA256:" + strings.Repeat("A", 43), "ssh-ed25519", nil
}

func cleanupReport(removed bool) string {
	report := `{"scope":"msboost","digest":"` + digest + `","items":[{"path":"/etc/msboost","kind":"directory"},{"path":"/root/直连.json","kind":"file"}],"removed":` + map[bool]string{true: "true", false: "false"}[removed] + `}`
	return "MSBOOST_CLEANUP=" + base64.StdEncoding.EncodeToString([]byte(report)) + "\n"
}

func (f *fakeServer) Run(ctx context.Context, _ executor.SSH, script string) ([]byte, error) {
	f.mu.Lock()
	f.scripts = append(f.scripts, script)
	f.mu.Unlock()
	if f.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if f.fail != nil {
		return nil, f.fail
	}
	config := base64.StdEncoding.EncodeToString([]byte(nodeConfig))
	switch {
	case strings.Contains(script, "MSBOOST_READY=1"):
		return []byte("MSBOOST_READY=1\n"), nil
	case strings.Contains(script, "MSBOOST_FORCE_FRESH=1"):
		return []byte("MSBOOST_BBR=enabled\nMSBOOST_STEP=Installing required packages\nMSBOOST_CONFIG=" + config + "\n"), nil
	case strings.Contains(script, "MSBOOST_INSTALLED"):
		return []byte("MSBOOST_OS=debian 12\nMSBOOST_BBR=bbr\nMSBOOST_INSTALLED=1\nMSBOOST_SERVICE=active\nMSBOOST_PROBE=active\nMSBOOST_ENGINE=v1.19.32\nMSBOOST_CONFIG=" + config + "\n"), nil
	case strings.Contains(script, "not_installed"):
		return []byte("MSBOOST_SERVICE=active\nMSBOOST_PROBE=active\nMSBOOST_CONFIG=" + config + "\n"), nil
	case strings.Contains(script, "MSBOOST_CLEANUP_PY"):
		return []byte(cleanupReport(!strings.Contains(script, base64.StdEncoding.EncodeToString([]byte("cleanup-preview"))))), nil
	}
	return nil, errors.New("unexpected script")
}

type harness struct {
	t       *testing.T
	runner  *Runner
	layout  store.Layout
	list    *servers.Store
	server  servers.Server
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
	h.server, err = h.list.Add(servers.Server{Name: "东京节点", Host: "8.8.8.8", Port: 22, AuthMethod: "password", Fingerprint: "SHA256:" + strings.Repeat("A", 43)})
	if err != nil {
		t.Fatal(err)
	}
	h.runner, err = New(h.layout, h.list, h.log, func() store.Settings { return h.setting }, func(e Event) { h.events <- e })
	if err != nil {
		t.Fatal(err)
	}
	h.runner.newEngine = func(store.Settings) *executor.Engine {
		return &executor.Engine{Remote: h.fake, TCPProbe: func(context.Context, string, int) string { return "reachable" }}
	}
	return h
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
	ev, err := h.runner.Start(Deploy, h.server.ID, Secret{Password: sshPassword})
	if err != nil {
		t.Fatal(err)
	}
	final, steps := h.wait(ev.ID)
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
	ev, _ = h.runner.Start(Deploy, h.server.ID, Secret{Password: sshPassword})
	if final, _ = h.wait(ev.ID); final.Outcome.ConfigFile != "东京节点.json" {
		t.Fatalf("redeploy wrote %s", final.Outcome.ConfigFile)
	}
}

func TestOneTaskPerServerAndCancel(t *testing.T) {
	h := newHarness(t)
	h.fake.block = true
	ev, err := h.runner.Start(Deploy, h.server.ID, Secret{Password: sshPassword})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.runner.Start(Fetch, h.server.ID, Secret{Password: sshPassword}); err == nil {
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
	ev, _ := h.runner.Start(Deploy, h.server.ID, Secret{Password: sshPassword})
	h.wait(ev.ID)

	preview, err := h.runner.Start(UninstallPreview, h.server.ID, Secret{Password: sshPassword})
	if err != nil {
		t.Fatal(err)
	}
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
	if final.State != "succeeded" || !strings.Contains(final.Outcome.Message, "东京节点.json 已经失效") {
		t.Fatalf("uninstall %+v %+v", final, final.Outcome)
	}
	last := h.fake.scripts[len(h.fake.scripts)-1]
	if !strings.Contains(last, base64.StdEncoding.EncodeToString([]byte(digest))) {
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
	ev, _ := h.runner.Start(Fetch, h.server.ID, Secret{Password: sshPassword})
	final, _ := h.wait(ev.ID)
	if final.State != "failed" || final.Outcome.ErrorCode != "ssh_timeout" || strings.Contains(final.Outcome.NextStep, "执行机") || !strings.Contains(final.Outcome.NextStep, "SSH 代理") {
		t.Fatalf("outcome %+v", final.Outcome)
	}
	h.setting.Proxy.Enabled = true
	ev, _ = h.runner.Start(Fetch, h.server.ID, Secret{Password: sshPassword})
	if final, _ = h.wait(ev.ID); final.Outcome.NextStep != proxyHint {
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
	ev, _ := h.runner.Start(Fetch, h.server.ID, Secret{Password: sshPassword})
	final, _ := h.wait(ev.ID)
	if final.Outcome.ConfigPath != filepath.Join(launcher, "configs", "东京节点.json") {
		t.Fatalf("saved to %s", final.Outcome.ConfigPath)
	}
}

func TestStatusUpdatesTheServer(t *testing.T) {
	h := newHarness(t)
	ev, _ := h.runner.Start(Status, h.server.ID, Secret{Password: sshPassword})
	final, _ := h.wait(ev.ID)
	if final.State != "succeeded" || final.Outcome.Node == nil || final.Outcome.Node.Engine != "v1.19.32" || final.Outcome.Node.OS != "Debian 12" {
		t.Fatalf("status %+v", final.Outcome)
	}
	if len(h.runner.Records()) != 0 {
		t.Fatal("status checks are not recorded")
	}
}

func TestSecretNeeded(t *testing.T) {
	h := newHarness(t)
	if got, err := h.runner.SecretNeeded(h.server.ID); got != "password" || err != nil {
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
		if got, err := h.runner.SecretNeeded(sv.ID); got != want || err != nil {
			t.Errorf("%s key: %q %v", name, got, err)
		}
	}
	h.list.Update(h.server.ID, func(v *servers.Server) error { v.KeyPath = filepath.Join(t.TempDir(), "junk"); return nil })
	if _, err := h.runner.SecretNeeded(h.server.ID); err == nil {
		t.Fatal("accepted a broken key file")
	}
	if _, err := h.runner.Start(Fetch, h.server.ID, Secret{}); err == nil {
		t.Fatal("started with an unreadable key")
	}
}
