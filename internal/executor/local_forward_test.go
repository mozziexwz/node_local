package executor

import (
	"context"
	"strings"
	"testing"
)

func forwardRemote() *fakeRemote {
	return &fakeRemote{run: func(_ SSH, script string) ([]byte, error) {
		switch {
		case script == "uname -m\n":
			return []byte("x86_64\n"), nil
		case strings.Contains(script, "MSBOOST_READY=1"):
			return []byte("MSBOOST_READY=1\n"), nil
		}
		return []byte("MSBOOST_BBR=enabled\nMSBOOST_RELAY_PORT=41877\n"), nil
	}}
}

func forwardEngine(remote Remote) *Engine {
	return &Engine{Remote: remote, TCPProbe: func(context.Context, string, int) string { return "reachable" },
		GostAMD64URL: "https://github.com/go-gost/gost/releases/download/v3.3.0/gost.tar.gz", GostAMD64SHA256: strings.Repeat("a", 64)}
}

func TestForwardRelayInstallsAManagedRelay(t *testing.T) {
	remote := forwardRemote()
	job := Job{ID: "f1", Request: Request{SSH: localTestSSH(), RateMbps: 3}}
	r := forwardEngine(remote).ForwardRelay(context.Background(), job, "9.9.9.9", 30512)
	if r.State != "succeeded" || len(r.Hops) != 1 || r.Hops[0] != (Hop{FromHost: "8.8.8.8", FromPort: 41877, ToHost: "9.9.9.9", ToPort: 30512}) || r.Health.PublicTCP != "reachable" {
		t.Fatalf("result %+v", r)
	}
	install := remote.scripts[len(remote.scripts)-1]
	for _, want := range []string{encodedAssignment("target_host", "9.9.9.9"), encodedAssignment("target_port", "30512"),
		encodedAssignment("task_id", "f1r"), encodedAssignment("rate_bytes", "375000"), encodedAssignment("meta_source", "")} {
		if !strings.Contains(install, want) {
			t.Fatalf("install script lacks %q", want)
		}
	}
	if !strings.Contains(r.Message, "3 Mbps") {
		t.Fatalf("message %q", r.Message)
	}
}

func TestForwardRelayRefusesBadTargets(t *testing.T) {
	for name, tc := range map[string]struct {
		host string
		port int
	}{
		"private address": {"192.168.1.10", 30512},
		"itself":          {"8.8.8.8", 30512},
		"bad port":        {"9.9.9.9", 0},
	} {
		remote := forwardRemote()
		r := forwardEngine(remote).ForwardRelay(context.Background(), Job{ID: "f1", Request: Request{SSH: localTestSSH()}}, tc.host, tc.port)
		if r.ErrorCode != "invalid_target" || len(remote.scripts) != 0 {
			t.Errorf("%s: %+v after %d scripts", name, r, len(remote.scripts))
		}
	}
	r := forwardEngine(forwardRemote()).ForwardRelay(context.Background(), Job{ID: "f1", Request: Request{SSH: localTestSSH(), RateMbps: 11}}, "9.9.9.9", 1)
	if r.State != "failed" {
		t.Fatalf("rate 11 accepted: %+v", r)
	}
}
