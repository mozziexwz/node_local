// Package store owns the portable data beside the executable:
//
//	configs\             node configurations for the launcher; when the launcher
//	                     is known, new configurations go to its configs\ instead
//	local\servers.json   saved servers: addresses and pinned host keys, never passwords
//	local\tasks.json     task records, without secrets or configuration contents
//	local\settings.json  SSH proxy and launcher location
//	local\logs\          daily log files
//	local\webview\       WebView2 cache
//
// Nothing here collides with the launcher's own files (configs\ is shared on
// purpose), so both programs can live in one folder.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// LauncherExe is the launcher's file name. When it sits beside this program,
// both share configs\.
const LauncherExe = "MSBOOST-Launcher.exe"

// Layout locates the data folders.
type Layout struct {
	Root     string
	Configs  string
	Local    string
	Logs     string
	Webview  string
	Servers  string
	Tasks    string
	Settings string
}

// NewLayout returns the layout rooted at dir.
func NewLayout(dir string) Layout {
	local := filepath.Join(dir, "local")
	return Layout{
		Root:     dir,
		Configs:  filepath.Join(dir, "configs"),
		Local:    local,
		Logs:     filepath.Join(local, "logs"),
		Webview:  filepath.Join(local, "webview"),
		Servers:  filepath.Join(local, "servers.json"),
		Tasks:    filepath.Join(local, "tasks.json"),
		Settings: filepath.Join(local, "settings.json"),
	}
}

// ExecutableLayout returns the layout beside the running executable.
func ExecutableLayout() (Layout, error) {
	exe, err := os.Executable()
	if err != nil {
		return Layout{}, err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return NewLayout(filepath.Dir(exe)), nil
}

// Ensure creates the folders this program writes to. configs\ is created
// when the first configuration is saved.
func (l Layout) Ensure() error {
	for _, dir := range []string{l.Local, l.Logs} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("无法创建数据目录 %s：%w", dir, err)
		}
	}
	return nil
}

// Settings are the user's choices, kept in local\settings.json.
type Settings struct {
	Version int   `json:"version"`
	Proxy   Proxy `json:"proxy"`
	// LauncherDir is the launcher's folder when it is not beside this program.
	LauncherDir string `json:"launcherDir,omitempty"`
}

// Proxy routes SSH connections through a local SOCKS5 proxy, for networks
// where direct SSH to the VPS is unreliable.
type Proxy struct {
	Enabled bool   `json:"enabled"`
	Host    string `json:"host"`
	Port    int    `json:"port"`
}

// Address is host:port for dialing.
func (p Proxy) Address() string { return net.JoinHostPort(p.Host, strconv.Itoa(p.Port)) }

// DefaultSettings suggests v2rayN's default local SOCKS port, switched off.
func DefaultSettings() Settings {
	return Settings{Version: 1, Proxy: Proxy{Host: "127.0.0.1", Port: 10808}}
}

var hostPattern = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.-]{0,251}[A-Za-z0-9])?$`)

// Validate checks the values a user can type.
func (s Settings) Validate() error {
	p := s.Proxy
	if net.ParseIP(p.Host) == nil && !hostPattern.MatchString(p.Host) {
		return errors.New("代理地址无效")
	}
	if p.Port < 1 || p.Port > 65535 {
		return errors.New("代理端口必须为 1–65535")
	}
	return nil
}

// LoadSettings reads the settings. A missing file gives the defaults; a
// broken one gives the defaults plus the reason.
func (l Layout) LoadSettings() (Settings, error) {
	raw, err := os.ReadFile(l.Settings)
	if errors.Is(err, os.ErrNotExist) {
		return DefaultSettings(), nil
	}
	if err != nil {
		return DefaultSettings(), fmt.Errorf("无法读取设置：%w", err)
	}
	s := DefaultSettings()
	if err := json.Unmarshal(raw, &s); err != nil {
		return DefaultSettings(), errors.New("设置文件已损坏，已使用默认设置")
	}
	if err := s.Validate(); err != nil {
		return DefaultSettings(), fmt.Errorf("设置无效（%v），已使用默认设置", err)
	}
	return s, nil
}

// SaveSettings validates and stores the settings.
func (l Layout) SaveSettings(s Settings) error {
	if err := s.Validate(); err != nil {
		return err
	}
	s.Version = 1
	return WriteJSON(l.Settings, s)
}

// Launcher is where the MSBOOST launcher was found.
type Launcher struct {
	Dir    string `json:"dir"`
	Beside bool   `json:"beside"` // in the same folder as this program
}

// FindLauncher looks beside this program first, then in the saved folder.
// The second result is false when neither holds the launcher.
func (l Layout) FindLauncher(s Settings) (Launcher, bool) {
	if isFile(filepath.Join(l.Root, LauncherExe)) {
		return Launcher{Dir: l.Root, Beside: true}, true
	}
	if s.LauncherDir != "" && isFile(filepath.Join(s.LauncherDir, LauncherExe)) {
		return Launcher{Dir: s.LauncherDir}, true
	}
	return Launcher{}, false
}

// ConfigsDir is where new configurations go: the launcher's configs\ when the
// launcher is known, otherwise configs\ beside this program.
func (l Layout) ConfigsDir(s Settings) string {
	if la, ok := l.FindLauncher(s); ok {
		return filepath.Join(la.Dir, "configs")
	}
	return l.Configs
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

var unsafeName = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f\x7f]`)
var reservedName = regexp.MustCompile(`(?i)^(con|prn|aux|nul|com[1-9]|lpt[1-9])(\.|$)`)

// ConfigName turns a remark into a configuration file name with the website's
// rules: characters Windows forbids become "_", at most 60 characters, no
// trailing dots or spaces, and reserved device names are replaced.
func ConfigName(base string) string {
	name := unsafeName.ReplaceAllString(base, "_")
	if r := []rune(name); len(r) > 60 {
		name = string(r[:60])
	}
	name = strings.TrimRight(name, ". ")
	if name == "" || reservedName.MatchString(name) {
		name = "MSBOOST配置"
	}
	return name + ".json"
}

// FreeConfigName returns name when it is free in dir or equals own (the file
// this server wrote before, which a new configuration replaces); otherwise
// the first free "name-2.json", "name-3.json", ….
func FreeConfigName(dir, name, own string) string {
	if strings.EqualFold(name, own) || !exists(filepath.Join(dir, name)) {
		return name
	}
	stem := strings.TrimSuffix(name, ".json")
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d.json", stem, i)
		if strings.EqualFold(candidate, own) || !exists(filepath.Join(dir, candidate)) {
			return candidate
		}
	}
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// SaveConfig writes a configuration into dir and returns its path.
func SaveConfig(dir, name string, data []byte) (string, error) {
	// Either slash is a separator on Windows, whatever the build platform.
	if name != filepath.Base(name) || strings.ContainsAny(name, `/\`) || !strings.HasSuffix(name, ".json") {
		return "", fmt.Errorf("配置文件名无效：%q", name)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("无法创建配置文件夹 %s：%w", dir, err)
	}
	path := filepath.Join(dir, name)
	if err := WriteFileAtomic(path, data); err != nil {
		return "", fmt.Errorf("无法保存配置 %s：%w", path, err)
	}
	return path, nil
}

// WriteJSON stores v as indented JSON, atomically.
func WriteJSON(path string, v any) error {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomic(path, append(raw, '\n'))
}

// WriteFileAtomic writes a temporary file beside path and renames it, so a
// crash never leaves a half-written file.
func WriteFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	_, err = tmp.Write(data)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(name, path)
	}
	if err != nil {
		os.Remove(name)
	}
	return err
}
