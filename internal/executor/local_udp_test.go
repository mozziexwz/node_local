package executor

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
)

func localTestConfigUDP(host string) string {
	return strings.Replace(localTestConfig(host), `{"port":30000,"protocol":"TCP"}`, `{"port":30000,"protocol":"TCP"},{"port":30000,"protocol":"UDP"}`, 1)
}

func TestUpgradeOpensUDPAndSaysTheFileMustBeReimported(t *testing.T) {
	remote := upgradeRemote(localTestConfig("8.8.8.8"), localTestConfigUDP("8.8.8.8"), true)
	var probed []byte
	e := &Engine{Remote: remote, TCPProbe: func(context.Context, string, int) string { return "reachable" }, UDPProbe: func(_ context.Context, config []byte) string {
		probed = config
		return "reachable"
	}}
	r := e.UpgradeWithProgress(context.Background(), Job{ID: "u1", Request: Request{SSH: localTestSSH()}, Script: localTestAsset()}, func(Step) {})
	if r.State != "succeeded" || r.Health.PublicUDP != "reachable" || string(probed) != localTestConfigUDP("8.8.8.8") {
		t.Fatalf("unexpected result: %+v", r)
	}
	if !strings.Contains(r.Message, "开启了 UDP") || !strings.Contains(r.Message, "重新导入") {
		t.Fatalf("a node that gained UDP must say the new file is needed: %s", r.Message)
	}
	if !strings.Contains(remote.scripts[2], "MSBOOST_UDP=1 bash \"$work/installer.sh\" "+progressPipe) {
		t.Fatal("an upgrade must ask the installer for UDP")
	}
	// Without a new UDP binding the old file keeps working and nothing is said.
	same := upgradeRemote(localTestConfigUDP("8.8.8.8"), localTestConfigUDP("8.8.8.8"), true)
	e.Remote = same
	if r := e.UpgradeWithProgress(context.Background(), Job{ID: "u2", Request: Request{SSH: localTestSSH()}, Script: localTestAsset()}, func(Step) {}); r.State != "succeeded" || strings.Contains(r.Message, "开启了 UDP") {
		t.Fatalf("unchanged UDP node: %+v", r)
	}
}

func TestFetchConfigChecksPublicUDP(t *testing.T) {
	config := localTestConfigUDP("8.8.8.8")
	remote := &fakeRemote{run: func(_ SSH, script string) ([]byte, error) {
		return []byte("MSBOOST_SERVICE=active\nMSBOOST_PROBE=active\nMSBOOST_CONFIG=" + base64.StdEncoding.EncodeToString([]byte(config)) + "\n"), nil
	}}
	e := &Engine{Remote: remote, TCPProbe: func(context.Context, string, int) string { return "reachable" }, UDPProbe: func(context.Context, []byte) string { return "unreachable" }}
	if r := e.FetchConfig(context.Background(), localTestSSH()); r.State != "succeeded" || r.Health.PublicUDP != "unreachable" {
		t.Fatalf("unexpected result: %+v", r)
	}
}
