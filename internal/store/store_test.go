package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigNameFollowsTheWebsite(t *testing.T) {
	for in, want := range map[string]string{
		"东京节点":                  "东京节点.json",
		`a<b>c:d"e/f\g|h?i*j`:   "a_b_c_d_e_f_g_h_i_j.json",
		"tab\there":             "tab_here.json",
		"trailing. . ":          "trailing.json",
		"":                      "MSBOOST配置.json",
		"...":                   "MSBOOST配置.json",
		"CON":                   "MSBOOST配置.json",
		"nul.txt":               "MSBOOST配置.json",
		"com1":                  "MSBOOST配置.json",
		"console":               "console.json",
		strings.Repeat("节", 70): strings.Repeat("节", 60) + ".json",
	} {
		if got := ConfigName(in); got != want {
			t.Errorf("ConfigName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFreeConfigName(t *testing.T) {
	dir := t.TempDir()
	if got := FreeConfigName(dir, "东京.json", ""); got != "东京.json" {
		t.Fatalf("free name changed: %s", got)
	}
	os.WriteFile(filepath.Join(dir, "东京.json"), []byte("{}"), 0o644)
	if got := FreeConfigName(dir, "东京.json", "东京.json"); got != "东京.json" {
		t.Fatalf("a server's own file must be replaced in place, got %s", got)
	}
	if got := FreeConfigName(dir, "东京.json", "别的.json"); got != "东京-2.json" {
		t.Fatalf("another server's file must be kept, got %s", got)
	}
	os.WriteFile(filepath.Join(dir, "东京-2.json"), []byte("{}"), 0o644)
	if got := FreeConfigName(dir, "东京.json", ""); got != "东京-3.json" {
		t.Fatalf("got %s", got)
	}
}

func TestSaveConfigRejectsPaths(t *testing.T) {
	dir := t.TempDir()
	for _, bad := range []string{`..\x.json`, "sub/x.json", "x.txt"} {
		if _, err := SaveConfig(dir, bad, []byte("{}")); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	path, err := SaveConfig(filepath.Join(dir, "configs"), "好.json", []byte(`{"a":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); string(data) != `{"a":1}` {
		t.Fatalf("saved %q", data)
	}
}

func TestSettingsRoundTripAndFallback(t *testing.T) {
	l := NewLayout(t.TempDir())
	if err := l.Ensure(); err != nil {
		t.Fatal(err)
	}
	s, err := l.LoadSettings()
	if err != nil || s != DefaultSettings() {
		t.Fatalf("missing file: %+v %v", s, err)
	}
	s.Proxy = Proxy{Enabled: true, Host: "127.0.0.1", Port: 7890}
	s.LauncherDir = `D:\MSBOOST`
	if err := l.SaveSettings(s); err != nil {
		t.Fatal(err)
	}
	if got, err := l.LoadSettings(); err != nil || got != s {
		t.Fatalf("round trip: %+v %v", got, err)
	}
	if err := l.SaveSettings(Settings{Proxy: Proxy{Host: "bad host", Port: 1}}); err == nil {
		t.Fatal("accepted an invalid proxy host")
	}
	os.WriteFile(l.Settings, []byte("{broken"), 0o644)
	if got, err := l.LoadSettings(); err == nil || got != DefaultSettings() {
		t.Fatalf("broken file: %+v %v", got, err)
	}
}

func TestLauncherDiscovery(t *testing.T) {
	root := t.TempDir()
	l := NewLayout(root)
	other := t.TempDir()
	s := Settings{LauncherDir: other}
	if _, ok := l.FindLauncher(s); ok {
		t.Fatal("found a launcher that does not exist")
	}
	if got := l.ConfigsDir(s); got != filepath.Join(root, "configs") {
		t.Fatalf("configs dir %s", got)
	}
	os.WriteFile(filepath.Join(other, LauncherExe), nil, 0o644)
	if la, ok := l.FindLauncher(s); !ok || la.Beside || la.Dir != other {
		t.Fatalf("saved folder: %+v %v", la, ok)
	}
	os.WriteFile(filepath.Join(root, LauncherExe), nil, 0o644)
	if la, ok := l.FindLauncher(s); !ok || !la.Beside {
		t.Fatalf("beside: %+v %v", la, ok)
	}
	if got := l.ConfigsDir(s); got != filepath.Join(root, "configs") {
		t.Fatalf("configs dir %s", got)
	}
}
