package executor

import (
	"context"
	"strings"
	"testing"
)

func forwardRemote() *fakeRemote { return forwardRemoteUDP("silent", true) }

// forwardRemoteUDP answers the UDP target check with verdict; bind says the
// relay install binds UDP when it is asked to.
func forwardRemoteUDP(verdict string, bind bool) *fakeRemote {
	return &fakeRemote{run: func(_ SSH, script string) ([]byte, error) {
		switch {
		case script == "uname -m\n":
			return []byte("x86_64\n"), nil
		case strings.Contains(script, "MSBOOST_READY=1"):
			return []byte("MSBOOST_READY=1\n"), nil
		case strings.Contains(script, "MSBOOST_UDP_TARGET"):
			return []byte("MSBOOST_UDP_TARGET=" + verdict + "\n"), nil
		case bind && strings.Contains(script, encodedAssignment("udp", "1")):
			return []byte("MSBOOST_BBR=enabled\nMSBOOST_RELAY_PORT=41877\nMSBOOST_RELAY_UDP=1\n"), nil
		}
		return []byte("MSBOOST_BBR=enabled\nMSBOOST_RELAY_PORT=41877\n"), nil
	}}
}

func forwardEngine(remote Remote) *Engine {
	return &Engine{Remote: remote, TCPProbe: func(context.Context, string, int) string { return "reachable" },
		GostAMD64URL: "https://github.com/go-gost/gost/releases/download/v3.3.0/gost.tar.gz", GostAMD64SHA256: strings.Repeat("a", 64)}
}

// scriptWith is the first script sent that contains s.
func scriptWith(scripts []string, s string) string {
	for _, script := range scripts {
		if strings.Contains(script, s) {
			return script
		}
	}
	return ""
}

func TestForwardRelayInstallsAManagedRelay(t *testing.T) {
	remote := forwardRemote()
	job := Job{ID: "f1", Request: Request{SSH: localTestSSH(), RateMbps: 3}}
	r := forwardEngine(remote).ForwardRelay(context.Background(), job, "9.9.9.9", 30512, false)
	if r.State != "succeeded" || len(r.Hops) != 1 || r.Hops[0] != (Hop{FromHost: "8.8.8.8", FromPort: 41877, ToHost: "9.9.9.9", ToPort: 30512}) || r.Health.PublicTCP != "reachable" {
		t.Fatalf("result %+v", r)
	}
	install := remote.scripts[len(remote.scripts)-1]
	for _, want := range []string{encodedAssignment("target_host", "9.9.9.9"), encodedAssignment("target_port", "30512"),
		encodedAssignment("task_id", "f1r"), encodedAssignment("rate_bytes", "375000"), encodedAssignment("meta_source", ""), encodedAssignment("udp", "0")} {
		if !strings.Contains(install, want) {
			t.Fatalf("install script lacks %q", want)
		}
	}
	if !strings.Contains(r.Message, "3 Mbps") || scriptWith(remote.scripts, "MSBOOST_UDP_TARGET") != "" {
		t.Fatalf("message %q, or the UDP port checked for a TCP-only node", r.Message)
	}
}

func TestForwardRelayForwardsUDPOnceTheTargetStaysSilent(t *testing.T) {
	remote := forwardRemote()
	r := forwardEngine(remote).ForwardRelay(context.Background(), Job{ID: "f1", Request: Request{SSH: localTestSSH()}}, "9.9.9.9", 30512, true)
	if r.State != "succeeded" || !r.Hops[0].UDP || !strings.Contains(r.Message, "TCP 和 UDP 都能用") {
		t.Fatalf("result %+v", r)
	}
	check := scriptWith(remote.scripts, "MSBOOST_UDP_TARGET")
	if !strings.Contains(check, encodedAssignment("target_host", "9.9.9.9")) || !strings.Contains(check, encodedAssignment("target_port", "30512")) {
		t.Fatal("the relay server must check the target's UDP port first")
	}
	if install := scriptWith(remote.scripts, encodedAssignment("task_id", "f1r")); !strings.Contains(install, encodedAssignment("udp", "1")) {
		t.Fatal("the relay was installed without UDP")
	}
}

func TestForwardRelayKeepsUDPOff(t *testing.T) {
	for name, tc := range map[string]struct {
		verdict string
		bind    bool
		port    int
		checked bool
		why     string
	}{
		"below 1024":     {"silent", true, 1000, false, ""},
		"answered":       {"answered", true, 30512, true, "回应了探测"},
		"refused":        {"refused", true, 30512, true, "没开"},
		"check failed":   {"", true, 30512, true, "没能检查"},
		"not bound":      {"silent", false, 30512, true, "没能开 UDP"},
		"unknown answer": {"maybe", true, 30512, true, "没能检查"},
	} {
		remote := forwardRemoteUDP(tc.verdict, tc.bind)
		r := forwardEngine(remote).ForwardRelay(context.Background(), Job{ID: "f1", Request: Request{SSH: localTestSSH()}}, "9.9.9.9", tc.port, true)
		if r.State != "succeeded" || r.Hops[0].UDP || (scriptWith(remote.scripts, "MSBOOST_UDP_TARGET") != "") != tc.checked {
			t.Errorf("%s: %+v", name, r)
			continue
		}
		if tc.why != "" && (!strings.Contains(r.Message, "只走 TCP") || !strings.Contains(r.Message, tc.why)) {
			t.Errorf("%s: message %q", name, r.Message)
		}
		if tc.verdict != "silent" && !strings.Contains(scriptWith(remote.scripts, encodedAssignment("task_id", "f1r")), encodedAssignment("udp", "0")) {
			t.Errorf("%s: UDP asked of the relay", name)
		}
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
		r := forwardEngine(remote).ForwardRelay(context.Background(), Job{ID: "f1", Request: Request{SSH: localTestSSH()}}, tc.host, tc.port, true)
		if r.ErrorCode != "invalid_target" || len(remote.scripts) != 0 {
			t.Errorf("%s: %+v after %d scripts", name, r, len(remote.scripts))
		}
	}
	r := forwardEngine(forwardRemote()).ForwardRelay(context.Background(), Job{ID: "f1", Request: Request{SSH: localTestSSH(), RateMbps: 11}}, "9.9.9.9", 1, false)
	if r.State != "failed" {
		t.Fatalf("rate 11 accepted: %+v", r)
	}
}
