package executor

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// A sync that adds an installer stage must add its step too.
func TestEveryInstallerNoteHasAStep(t *testing.T) {
	installer, err := os.ReadFile(filepath.Join("..", "..", "installers", "node", "msboost.sh"))
	if err != nil {
		t.Fatal(err)
	}
	notes := regexp.MustCompile(`\bnote "([^"]+)"`).FindAllStringSubmatch(string(installer), -1)
	if len(notes) < 8 {
		t.Fatalf("found only %d installer notes", len(notes))
	}
	for _, n := range notes {
		if _, ok := installerStep(n[1]); !ok {
			t.Errorf("installer note %q has no step in installerSteps", n[1])
		}
	}
}

func localTestAsset() Asset {
	data := []byte("#!/usr/bin/env bash\necho installer\n")
	sum := sha256.Sum256(data)
	return Asset{Data: data, SHA256: hex.EncodeToString(sum[:])}
}

// deployRemote answers the preflight and the deploy script like a server
// whose installer succeeded and announced two stages.
func deployRemote(config string) *fakeRemote {
	return &fakeRemote{run: func(_ SSH, script string) ([]byte, error) {
		if strings.Contains(script, "MSBOOST_READY=1") {
			return []byte("MSBOOST_READY=1\n"), nil
		}
		return []byte("MSBOOST_BBR=enabled\nMSBOOST_STEP=Installing required packages\nMSBOOST_STEP=Downloading and verifying the pinned engine v1.19.32\nMSBOOST_CONFIG=" +
			base64.StdEncoding.EncodeToString([]byte(config)) + "\n"), nil
	}}
}

var randomCredentials = regexp.MustCompile(`(node_user|node_pass)=\$\(printf '%s' '[A-Za-z0-9+/=]*'`)

// DeployWithProgress must send exactly engine.go's deploy, except that the
// installer's output also passes through the progress pipe.
func TestDeployWithProgressSendsTheEngineDeploy(t *testing.T) {
	job := Job{ID: "a1", Request: Request{Kind: "deploy", Mode: "fresh", SSH: localTestSSH()}, Script: localTestAsset()}
	upstream := deployRemote(localTestConfig("8.8.8.8"))
	probe := func(context.Context, string, int) string { return "reachable" }
	want := (&Engine{Remote: upstream, TCPProbe: probe}).Execute(context.Background(), job)
	local := deployRemote(localTestConfig("8.8.8.8"))
	var steps []Step
	got := (&Engine{Remote: local, TCPProbe: probe}).DeployWithProgress(context.Background(), job, func(s Step) { steps = append(steps, s) })

	if want.State != "succeeded" || got.State != want.State || string(got.Config) != string(want.Config) || *got.Health != *want.Health || got.Message != want.Message {
		t.Fatalf("results differ:\nengine %+v\nlocal  %+v", want, got)
	}
	if len(local.scripts) != len(upstream.scripts) {
		t.Fatalf("local sent %d scripts, engine %d", len(local.scripts), len(upstream.scripts))
	}
	for i := range local.scripts {
		norm := func(s string) string { return randomCredentials.ReplaceAllString(s, "$1=X") }
		w := norm(upstream.scripts[i])
		g := norm(strings.Replace(local.scripts[i], progressPipe, `>"$work/private.log" 2>&1`, 1))
		if g != w {
			t.Fatalf("script %d differs from engine.go's deploy; resync DeployWithProgress", i)
		}
	}
	if !strings.Contains(local.scripts[1], progressPipe) {
		t.Fatal("deploy script lacks the progress pipe")
	}
	wantSteps := []Step{StepConnect, StepBBR, StepPackages, StepEngine, StepConfig}
	if len(steps) != len(wantSteps) {
		t.Fatalf("steps %v, want %v", steps, wantSteps)
	}
	for i := range steps {
		if steps[i] != wantSteps[i] {
			t.Fatalf("steps %v, want %v", steps, wantSteps)
		}
	}
}

func TestDeployWithProgressStopsAtFailures(t *testing.T) {
	job := Job{ID: "a1", Request: Request{Kind: "deploy", Mode: "fresh", SSH: localTestSSH()}, Script: localTestAsset()}
	bad := job
	bad.Script.SHA256 = strings.Repeat("0", 64)
	if r := (&Engine{Remote: deployRemote("")}).DeployWithProgress(context.Background(), bad, func(Step) {}); r.ErrorCode != "integrity_failed" {
		t.Fatalf("tampered installer: %+v", r)
	}
	other := deployRemote(localTestConfig("1.1.1.1"))
	if r := (&Engine{Remote: other, TCPProbe: func(context.Context, string, int) string { return "" }}).DeployWithProgress(context.Background(), job, func(Step) {}); r.State != "failed" || r.ErrorCode != "invalid_config" {
		t.Fatalf("config for another host: %+v", r)
	}
}

func bashPath(t *testing.T) string {
	t.Helper()
	bash, err := exec.LookPath("bash")
	if err != nil && runtime.GOOS == "windows" {
		bash = "C:/Program Files/Git/bin/bash.exe"
		if _, err = os.Stat(bash); err != nil {
			t.Skip("bash unavailable")
		}
	} else if err != nil {
		t.Skip("bash unavailable")
	}
	return bash
}

// The pipe forwards only the announcements, keeps everything in private.log
// and passes the installer's exit status on.
func TestProgressPipeForwardsNotesAndKeepsTheLog(t *testing.T) {
	bash := bashPath(t)
	for _, status := range []string{"0", "3"} {
		work := filepath.ToSlash(t.TempDir())
		script := "set -Eeuo pipefail\nwork='" + work + "'\n" +
			"printf '%s\\n' 'echo \"==> Installing required packages\"' 'echo noise' 'echo \"==> Deploying msboost transactionally\" >&2' 'exit " + status + "' > \"$work/installer.sh\"\n" +
			"bash \"$work/installer.sh\" " + progressPipe + "\n"
		cmd := exec.Command(bash, "-c", script)
		out, err := cmd.Output()
		got := strings.ReplaceAll(string(out), "\r", "")
		if want := "MSBOOST_STEP=Installing required packages\nMSBOOST_STEP=Deploying msboost transactionally\n"; got != want {
			t.Fatalf("status %s: forwarded %q, want %q", status, got, want)
		}
		if (status == "0") != (err == nil) {
			t.Fatalf("status %s: pipeline error %v", status, err)
		}
		log, _ := os.ReadFile(filepath.Join(work, "private.log"))
		if !strings.Contains(string(log), "noise") || !strings.Contains(string(log), "==> Installing") {
			t.Fatalf("private.log lacks the full output: %q", log)
		}
	}
}

// When this computer disconnects, the forwarding loop dies of SIGPIPE; the
// installer must still run to the end and keep writing private.log.
func TestProgressPipeSurvivesADisconnect(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("SIGPIPE semantics are checked on Linux")
	}
	bash := bashPath(t)
	work := t.TempDir()
	installer := "echo '==> Installing required packages'\nsleep 1\necho '==> Deploying msboost transactionally'\necho done > \"$1/finished\"\n"
	os.WriteFile(filepath.Join(work, "installer.sh"), []byte(installer), 0o755)
	script := "set -Eeuo pipefail\nwork='" + work + "'\n(bash \"$work/installer.sh\" \"$work\" " + progressPipe + ") | head -n 1 >/dev/null\nsleep 2\n"
	if out, err := exec.Command(bash, "-c", script).CombinedOutput(); err != nil {
		t.Logf("outer shell: %v %s", err, out) // the outer pipeline may report SIGPIPE
	}
	if _, err := os.Stat(filepath.Join(work, "finished")); err != nil {
		t.Fatal("the installer stopped when the progress reader went away")
	}
	log, _ := os.ReadFile(filepath.Join(work, "private.log"))
	if !strings.Contains(string(log), "Deploying msboost") {
		t.Fatalf("private.log stopped early: %q", log)
	}
}
