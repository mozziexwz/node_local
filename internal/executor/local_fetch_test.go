package executor

import (
	"context"
	"encoding/base64"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func localTestSSH() SSH {
	return SSH{Host: "8.8.8.8", Port: 22, User: "root", Password: "pw", Fingerprint: "SHA256:" + strings.Repeat("A", 43)}
}

func localTestConfig(host string) string {
	return `{"profiles":[{"profileName":"msboost","user":{"name":"msboost-a1","password":"secret"},"servers":[{"ipAddress":"` + host + `","domainName":"","portBindings":[{"port":30000,"protocol":"TCP"}]}],"mtu":1400}],"activeProfile":"msboost","rpcPort":8964,"socks5Port":10086}`
}

func TestFetchConfigReturnsTheNodeConfiguration(t *testing.T) {
	config := localTestConfig("8.8.8.8")
	remote := &fakeRemote{run: func(_ SSH, script string) ([]byte, error) {
		return []byte("MSBOOST_SERVICE=active\nMSBOOST_PROBE=active\nMSBOOST_CONFIG=" + base64.StdEncoding.EncodeToString([]byte(config)) + "\n"), nil
	}}
	var probed string
	e := &Engine{Remote: remote, TCPProbe: func(_ context.Context, host string, port int) string {
		probed = host
		return "reachable"
	}}
	r := e.FetchConfig(context.Background(), localTestSSH())
	if r.State != "succeeded" || string(r.Config) != config || r.Health == nil || r.Health.Service != "running" || r.Health.PublicTCP != "reachable" || probed != "8.8.8.8" {
		t.Fatalf("unexpected result: %+v", r)
	}
	if len(remote.scripts) != 1 || remote.scripts[0] != fetchConfigScript {
		t.Fatalf("fetch must run only the read-only script, ran %d", len(remote.scripts))
	}
}

func TestFetchConfigReportsStoppedServices(t *testing.T) {
	config := localTestConfig("8.8.8.8")
	remote := &fakeRemote{run: func(SSH, string) ([]byte, error) {
		return []byte("MSBOOST_SERVICE=active\nMSBOOST_PROBE=inactive\nMSBOOST_CONFIG=" + base64.StdEncoding.EncodeToString([]byte(config)) + "\n"), nil
	}}
	e := &Engine{Remote: remote, TCPProbe: func(context.Context, string, int) string { return "unreachable" }}
	if r := e.FetchConfig(context.Background(), localTestSSH()); r.State != "succeeded" || r.Health.Service != "stopped" {
		t.Fatalf("unexpected result: %+v", r)
	}
}

func TestFetchConfigFailures(t *testing.T) {
	for _, tc := range []struct {
		name, output, want string
		err                error
	}{
		{"not installed", "MSBOOST_ERROR_CODE=not_installed\nMSBOOST_ERROR_PHASE=config\n", "not_installed", errors.New("exit status 1")},
		{"other host", "MSBOOST_CONFIG=" + base64.StdEncoding.EncodeToString([]byte(localTestConfig("1.1.1.1"))) + "\n", "config_host_mismatch", nil},
		{"no config", "MSBOOST_SERVICE=active\n", "invalid_config", nil},
		{"broken config", "MSBOOST_CONFIG=" + base64.StdEncoding.EncodeToString([]byte(`{"profiles":[]}`)) + "\n", "invalid_config", nil},
	} {
		remote := &fakeRemote{run: func(SSH, string) ([]byte, error) { return []byte(tc.output), tc.err }}
		e := &Engine{Remote: remote, TCPProbe: func(context.Context, string, int) string { return "reachable" }}
		r := e.FetchConfig(context.Background(), localTestSSH())
		if r.State != "failed" || r.ErrorCode != tc.want || r.NextStep == "" || len(r.Config) != 0 {
			t.Errorf("%s: unexpected result %+v", tc.name, r)
		}
	}
}

func TestFetchConfigValidatesSSHFirst(t *testing.T) {
	remote := &fakeRemote{run: func(SSH, string) ([]byte, error) { t.Fatal("must not connect"); return nil, nil }}
	s := localTestSSH()
	s.Fingerprint = ""
	if r := (&Engine{Remote: remote}).FetchConfig(context.Background(), s); r.State != "failed" || r.ErrorCode != "invalid_request" {
		t.Fatalf("unexpected result: %+v", r)
	}
}

func TestFetchConfigScriptSyntax(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash unavailable")
	}
	cmd := exec.Command(bash, "-n")
	cmd.Stdin = strings.NewReader(fetchConfigScript)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("bash -n: %v %s", err, out)
	}
}
