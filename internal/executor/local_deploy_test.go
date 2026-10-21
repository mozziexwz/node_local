package executor

import (
	"context"
	"encoding/base64"
	"errors"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

// upgradeRemote answers the fetch, the preflight and the upgrade script. The
// node's configuration before and after the upgrade are given.
func upgradeRemote(before, after string, installed bool) *fakeRemote {
	return &fakeRemote{run: func(_ SSH, script string) ([]byte, error) {
		switch {
		case script == fetchConfigScript:
			if !installed {
				return []byte("MSBOOST_ERROR_CODE=not_installed\nMSBOOST_ERROR_PHASE=config\n"), errors.New("exit status 1")
			}
			return []byte("MSBOOST_SERVICE=active\nMSBOOST_PROBE=active\nMSBOOST_CONFIG=" + base64.StdEncoding.EncodeToString([]byte(before)) + "\n"), nil
		case strings.Contains(script, "MSBOOST_READY=1"):
			return []byte("MSBOOST_READY=1\n"), nil
		default:
			return []byte("MSBOOST_BBR=enabled\nMSBOOST_STEP=Deploying msboost transactionally\nMSBOOST_CONFIG=" + base64.StdEncoding.EncodeToString([]byte(after)) + "\n"), nil
		}
	}}
}

func TestUpgradeKeepsTheConfiguration(t *testing.T) {
	config := localTestConfig("8.8.8.8")
	remote := upgradeRemote(config, config, true)
	var steps []Step
	e := &Engine{Remote: remote, TCPProbe: func(context.Context, string, int) string { return "reachable" }}
	r := e.UpgradeWithProgress(context.Background(), Job{ID: "u1", Request: Request{SSH: localTestSSH()}, Script: localTestAsset()}, func(s Step) { steps = append(steps, s) })
	if r.State != "succeeded" || string(r.Config) != config || r.Health.PublicTCP != "reachable" || r.Health.BBR != "enabled" {
		t.Fatalf("unexpected result: %+v", r)
	}
	if len(remote.scripts) != 3 {
		t.Fatalf("sent %d scripts, want fetch, preflight and upgrade", len(remote.scripts))
	}
	upgrade := remote.scripts[2]
	if strings.Contains(upgrade, "MSBOOST_FORCE_FRESH") || strings.Contains(upgrade, "node_pass") {
		t.Fatal("an upgrade must not ask for new credentials")
	}
	if !strings.Contains(upgrade, encodedAssignment("MSBOOST_SERVER_IP", "8.8.8.8")) || !strings.Contains(upgrade, "bash \"$work/installer.sh\" "+progressPipe) {
		t.Fatal("upgrade script lacks the entry IP or the progress pipe")
	}
	if steps[0] != StepConnect || steps[len(steps)-1] != StepConfig {
		t.Fatalf("steps %v", steps)
	}
}

func TestUpgradeRefusesWithoutANodeOrWhenTheConfigurationChanges(t *testing.T) {
	config := localTestConfig("8.8.8.8")
	none := upgradeRemote(config, config, false)
	e := &Engine{Remote: none}
	if r := e.UpgradeWithProgress(context.Background(), Job{ID: "u1", Request: Request{SSH: localTestSSH()}, Script: localTestAsset()}, func(Step) {}); r.ErrorCode != "not_installed" || len(none.scripts) != 1 {
		t.Fatalf("no node: %+v after %d scripts", r, len(none.scripts))
	}
	changed := strings.Replace(config, `"password":"secret"`, `"password":"other"`, 1)
	moved := upgradeRemote(config, changed, true)
	e = &Engine{Remote: moved, TCPProbe: func(context.Context, string, int) string { return "reachable" }}
	if r := e.UpgradeWithProgress(context.Background(), Job{ID: "u1", Request: Request{SSH: localTestSSH()}, Script: localTestAsset()}, func(Step) {}); r.ErrorCode != "config_changed" || r.NextStep == "" || len(r.Config) != 0 {
		t.Fatalf("changed credentials: %+v", r)
	}
}

func TestNodeStatus(t *testing.T) {
	config := localTestConfig("8.8.8.8")
	remote := &fakeRemote{run: func(SSH, string) ([]byte, error) {
		return []byte("MSBOOST_OS=debian 12\nMSBOOST_BBR=bbr\nMSBOOST_INSTALLED=1\nMSBOOST_SERVICE=active\nMSBOOST_PROBE=active\nMSBOOST_ENGINE=Mihomo Meta v1.19.32 linux amd64 with go1.24.4\nMSBOOST_CONFIG=" +
			base64.StdEncoding.EncodeToString([]byte(config)) + "\n"), nil
	}}
	e := &Engine{Remote: remote, TCPProbe: func(_ context.Context, host string, port int) string {
		if host != "8.8.8.8" || port != 30000 {
			t.Errorf("probed %s:%d", host, port)
		}
		return "reachable"
	}}
	st, r := e.NodeStatus(context.Background(), localTestSSH())
	want := NodeStatus{OS: "Debian 12", BBR: "enabled", Installed: true, Service: "running", Engine: "v1.19.32", Entry: "8.8.8.8:30000", PublicTCP: "reachable", Config: []byte(config)}
	if r.State != "succeeded" || !reflect.DeepEqual(st, want) {
		t.Fatalf("status %+v result %+v", st, r)
	}
	if remote.scripts[0] != nodeStatusScript {
		t.Fatal("status must run only the read-only script")
	}

	bare := &fakeRemote{run: func(SSH, string) ([]byte, error) {
		return []byte("MSBOOST_OS=debian 13\nMSBOOST_BBR=cubic\nMSBOOST_INSTALLED=0\n"), nil
	}}
	st, r = (&Engine{Remote: bare}).NodeStatus(context.Background(), localTestSSH())
	if r.State != "succeeded" || st.Installed || st.OS != "Debian 13" || st.BBR != "off" {
		t.Fatalf("server without a node: %+v %+v", st, r)
	}

	auth := &fakeRemote{run: func(SSH, string) ([]byte, error) { return nil, diagnosticError("ssh_auth") }}
	if _, r = (&Engine{Remote: auth}).NodeStatus(context.Background(), localTestSSH()); r.ErrorCode != "ssh_auth" {
		t.Fatalf("auth failure: %+v", r)
	}
}

func TestLocalScriptsParse(t *testing.T) {
	bash := bashPath(t)
	for name, script := range map[string]string{"status": nodeStatusScript, "tail": "set -Eeuo pipefail\n" + installedTail, "pipe": "work=/tmp\ntrue " + progressPipe + "\n"} {
		cmd := exec.Command(bash, "-n")
		cmd.Stdin = strings.NewReader(script)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("%s: bash -n: %v %s", name, err, out)
		}
	}
}
