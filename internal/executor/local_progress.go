package executor

import (
	"context"
	"strings"
)

// Local edition: deploy and upgrade report progress as steps that the app
// shows in Chinese. The node installer announces each stage with a "==> "
// line; the deploy script forwards only those lines, as MSBOOST_STEP=
// markers. The rest of the installer's output stays in private.log on the
// server: it is English, names the upstream components and prints the node's
// credentials.

// Step is one stage of a deploy or upgrade, in order.
type Step int

const (
	StepConnect   Step = iota + 1 // connecting and checking the host key
	StepPreflight                 // checking for Debian 11+ with systemd
	StepBBR                       // tuning BBR
	StepPackages                  // installing packages
	StepClock                     // synchronising the clock
	StepEngine                    // downloading and verifying the engine
	StepRules                     // downloading the routing rules
	StepInstall                   // checking the configuration, installing the service
	StepSelfTest                  // self-tests and the local firewall
	StepConfig                    // reading back the client configuration
)

// installerSteps maps the installer's announcements (msboost.sh note calls)
// to steps. TestEveryInstallerNoteHasAStep keeps it complete after a sync.
var installerSteps = []struct {
	prefix string
	step   Step
}{
	{"Installing required packages", StepPackages},
	{"Setting ", StepClock},
	{"System clock synchronized", StepClock},
	{"Downloading and verifying ", StepEngine},
	{"Downloading the current GFW domain data", StepRules},
	{"Validating the staged configuration", StepInstall},
	{"Deploying msboost", StepInstall},
	{"Validating the loopback TCP probe", StepSelfTest},
	{"Running a local end-to-end proxy self-test", StepSelfTest},
	{"Updating any active local firewall", StepSelfTest},
}

func installerStep(note string) (Step, bool) {
	for _, s := range installerSteps {
		if strings.HasPrefix(note, s.prefix) {
			return s.step, true
		}
	}
	return 0, false
}

// progressPipe takes the place of the deploy script's ">private.log 2>&1".
// private.log still receives everything (the diagnostics read it). tee -p
// ignores a broken pipe, so if this computer disconnects the installer keeps
// writing private.log and finishes, exactly as without the pipe.
const progressPipe = `2>&1 | tee -p "$work/private.log" | while IFS= read -r msboost_line; do case "$msboost_line" in "==> "*) printf 'MSBOOST_STEP=%s\n' "${msboost_line#==> }" ;; esac; done`

// installedTail follows the installer in engine.go's deploy: both services
// must run, then the client configuration is printed.
const installedTail = "msboost_phase=service\nsystemctl is-active --quiet msboost.service\nsystemctl is-active --quiet msboost-tcp-probe.service\nmsboost_phase=config\nprintf 'MSBOOST_CONFIG='\nbase64 -w0 /root/直连.json\nprintf '\\n'\n"

// runWithProgress runs a script and reports its MSBOOST_STEP markers. A
// remote that cannot stream reports them when the script has ended.
func (e *Engine) runWithProgress(ctx context.Context, s SSH, script string, progress func(Step)) ([]byte, error) {
	report := func(line string) {
		if note, ok := strings.CutPrefix(line, "MSBOOST_STEP="); ok {
			if step, ok := installerStep(note); ok {
				progress(step)
			}
		}
	}
	if sr, ok := e.Remote.(StreamRemote); ok {
		return sr.RunStream(ctx, s, script, report)
	}
	out, err := e.Remote.Run(ctx, s, script)
	for _, line := range strings.Split(string(out), "\n") {
		report(strings.TrimRight(line, "\r"))
	}
	return out, err
}
