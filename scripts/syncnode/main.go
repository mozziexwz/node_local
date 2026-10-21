// Command syncnode copies the executor engine and the node installer from a
// tagged release of the website repository (mozziexwz/node) and records their
// origin in UPSTREAM.json. Synced files are never edited by hand; local
// additions live in internal/executor/local_*.go, which syncing leaves alone.
//
// Run it from the repository root:
//
//	go run ./scripts/syncnode -tag v2.7.0 [-source ..\MSBOOST.DE]
//	go run ./scripts/syncnode -check [-source path]
//
// -check regenerates every synced file from the commit in UPSTREAM.json and
// fails if anything differs, so CI can prove the copies are untouched.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	repository    = "https://github.com/mozziexwz/node"
	executorDir   = "internal/executor"
	installerPath = "installers/node/msboost.sh"
	gostEnvPath   = "deploy/agent-executor.env.example"
	gostJSONPath  = "installers/gost.json"
	manifestPath  = "UPSTREAM.json"
)

// Files of the executor package the local edition does not use: the website
// polling client, the administrator's SFTP backup helper, and the DD test that
// needs the reinstall script (DD is not part of the local edition yet).
var excluded = map[string]bool{
	"client.go":              true,
	"client_test.go":         true,
	"sftp_ownership.go":      true,
	"sftp_ownership_test.go": true,
	"unattended_test.go":     true,
}

type file struct {
	Path    string `json:"path"`
	From    string `json:"from"`
	SHA256  string `json:"sha256"`
	Derived bool   `json:"derived,omitempty"`
}

type manifest struct {
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
	Commit     string `json:"commit"`
	Files      []file `json:"files"`
}

type synced struct {
	file
	data []byte
}

func main() {
	tag := flag.String("tag", "", "website release tag to copy from, e.g. v2.7.0")
	source := flag.String("source", filepath.Join("..", "MSBOOST.DE"), "local clone of mozziexwz/node")
	check := flag.Bool("check", false, "verify the synced files against UPSTREAM.json instead of copying")
	flag.Parse()
	if err := run(*tag, *source, *check); err != nil {
		fmt.Fprintln(os.Stderr, "syncnode:", err)
		os.Exit(1)
	}
}

func run(tag, source string, check bool) error {
	mod, err := os.ReadFile("go.mod")
	if err != nil || !bytes.HasPrefix(mod, []byte("module github.com/mozziexwz/node_local\n")) {
		return errors.New("run from the node_local repository root")
	}
	if check {
		return verify(source)
	}
	if tag == "" {
		return errors.New("-tag is required")
	}
	commit, err := git(source, "rev-parse", "--verify", tag+"^{commit}")
	if err != nil {
		return err
	}
	files, err := collect(source, strings.TrimSpace(string(commit)))
	if err != nil {
		return err
	}
	keep := map[string]bool{}
	for _, f := range files {
		if err := os.MkdirAll(filepath.Dir(filepath.FromSlash(f.Path)), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.FromSlash(f.Path), f.data, 0o644); err != nil {
			return err
		}
		keep[f.Path] = true
	}
	stale, err := staleExecutorFiles(keep)
	if err != nil {
		return err
	}
	for _, p := range stale {
		if err := os.Remove(filepath.FromSlash(p)); err != nil {
			return err
		}
		fmt.Println("removed", p)
	}
	m := manifest{Repository: repository, Tag: tag, Commit: strings.TrimSpace(string(commit))}
	for _, f := range files {
		m.Files = append(m.Files, f.file)
	}
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(manifestPath, append(raw, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Printf("synced %d files from %s (%s)\n", len(files), tag, m.Commit[:12])
	return nil
}

// verify regenerates the synced files from the recorded commit and compares
// them with the working tree byte for byte.
func verify(source string) error {
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return err
	}
	var m manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return fmt.Errorf("%s: %w", manifestPath, err)
	}
	if tagged, err := git(source, "rev-parse", "--verify", m.Tag+"^{commit}"); err != nil || strings.TrimSpace(string(tagged)) != m.Commit {
		return fmt.Errorf("tag %s does not point to recorded commit %s", m.Tag, m.Commit)
	}
	files, err := collect(source, m.Commit)
	if err != nil {
		return err
	}
	var problems []string
	keep := map[string]bool{}
	recorded := map[string]file{}
	for _, f := range m.Files {
		recorded[f.Path] = f
	}
	for _, f := range files {
		keep[f.Path] = true
		if recorded[f.Path] != f.file {
			problems = append(problems, f.Path+": differs from UPSTREAM.json")
		}
		local, err := os.ReadFile(filepath.FromSlash(f.Path))
		if err != nil || !bytes.Equal(local, f.data) {
			problems = append(problems, f.Path+": differs from "+m.Tag)
		}
	}
	if len(m.Files) != len(files) {
		problems = append(problems, "UPSTREAM.json lists a different set of files")
	}
	stale, err := staleExecutorFiles(keep)
	if err != nil {
		return err
	}
	for _, p := range stale {
		problems = append(problems, p+": not from "+m.Tag+" and not a local_* file")
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "\n"))
	}
	fmt.Printf("%d synced files match %s (%s)\n", len(files), m.Tag, m.Commit[:12])
	return nil
}

func collect(source, commit string) ([]synced, error) {
	list, err := git(source, "ls-tree", "-r", "--name-only", commit, "--", executorDir)
	if err != nil {
		return nil, err
	}
	var out []synced
	for _, p := range strings.Split(strings.TrimSpace(string(list)), "\n") {
		name := path.Base(p)
		if path.Dir(p) != executorDir || excluded[name] || (path.Ext(name) != ".go" && path.Ext(name) != ".py") {
			continue
		}
		f, err := blob(source, commit, p, p)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	installer, err := blob(source, commit, installerPath, installerPath)
	if err != nil {
		return nil, err
	}
	out = append(out, installer)
	gost, err := gostPins(source, commit)
	if err != nil {
		return nil, err
	}
	out = append(out, gost)
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func blob(source, commit, from, to string) (synced, error) {
	data, err := git(source, "cat-file", "blob", commit+":"+from)
	if err != nil {
		return synced{}, err
	}
	return synced{file{Path: to, From: from, SHA256: digest(data)}, data}, nil
}

// gostPins turns the executor's pinned GOST archives (an env example on the
// website) into installers/gost.json.
func gostPins(source, commit string) (synced, error) {
	env, err := git(source, "cat-file", "blob", commit+":"+gostEnvPath)
	if err != nil {
		return synced{}, err
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(env), "\n") {
		if key, value, ok := strings.Cut(strings.TrimSpace(line), "="); ok && strings.HasPrefix(key, "GOST_") {
			values[key] = value
		}
	}
	url := regexp.MustCompile(`^https://github\.com/go-gost/gost/releases/download/v[0-9.]+/gost_[0-9.]+_linux_(amd64|arm64)\.tar\.gz$`)
	sum := regexp.MustCompile(`^[0-9a-f]{64}$`)
	pins := map[string]map[string]string{}
	for _, arch := range []string{"amd64", "arm64"} {
		u, s := values["GOST_"+strings.ToUpper(arch)+"_URL"], values["GOST_"+strings.ToUpper(arch)+"_SHA256"]
		if !url.MatchString(u) || !strings.HasSuffix(u, "_linux_"+arch+".tar.gz") || !sum.MatchString(s) {
			return synced{}, fmt.Errorf("%s: missing or invalid GOST pin for %s", gostEnvPath, arch)
		}
		pins[arch] = map[string]string{"url": u, "sha256": s}
	}
	data, err := json.MarshalIndent(pins, "", "  ")
	if err != nil {
		return synced{}, err
	}
	data = append(data, '\n')
	return synced{file{Path: gostJSONPath, From: gostEnvPath, SHA256: digest(data), Derived: true}, data}, nil
}

// staleExecutorFiles lists executor files that are neither synced nor local.
func staleExecutorFiles(keep map[string]bool) ([]string, error) {
	entries, err := os.ReadDir(filepath.FromSlash(executorDir))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var stale []string
	for _, e := range entries {
		p := executorDir + "/" + e.Name()
		if e.IsDir() || strings.HasPrefix(e.Name(), "local_") || keep[p] {
			continue
		}
		stale = append(stale, p)
	}
	return stale, nil
}

func git(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %v %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
