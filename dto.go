package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/mozziexwz/node_local/internal/servers"
	"github.com/mozziexwz/node_local/internal/store"
	"github.com/mozziexwz/node_local/internal/tasks"
)

// OverviewDTO is everything the pages need in one call.
type OverviewDTO struct {
	Servers    []ServerDTO     `json:"servers"`
	Running    []tasks.Event   `json:"running"`
	Proxy      store.Proxy     `json:"proxy"`
	Launcher   *store.Launcher `json:"launcher"` // nil when unknown
	ConfigsDir string          `json:"configsDir"`
	DataDir    string          `json:"dataDir"`
	Version    string          `json:"version"`
	Warnings   []string        `json:"warnings"`
}

// ServerDTO is a saved server for the server page.
type ServerDTO struct {
	ID          string                `json:"id"`
	Label       string                `json:"label"`
	Name        string                `json:"name"`
	Host        string                `json:"host"`
	Port        int                   `json:"port"`
	AuthMethod  string                `json:"authMethod"`
	KeyPath     string                `json:"keyPath"`
	Fingerprint string                `json:"fingerprint"`
	Algorithm   string                `json:"algorithm"`
	TrustedAt   int64                 `json:"trustedAt"`
	ConfigFile  string                `json:"configFile"`
	ConfigPath  string                `json:"configPath"` // "" when the file is gone
	InLauncher  bool                  `json:"inLauncher"` // the file is where the launcher reads it
	Node        *servers.NodeState    `json:"node"`
	Relays      *servers.RelaySummary `json:"relays"`
}

func toServerDTO(sv servers.Server, configsDir string, launcherFound bool) ServerDTO {
	dto := ServerDTO{ID: sv.ID, Label: sv.Label(), Name: sv.Name, Host: sv.Host, Port: sv.Port, AuthMethod: sv.AuthMethod,
		KeyPath: sv.KeyPath, Fingerprint: sv.Fingerprint, Algorithm: sv.Algorithm, TrustedAt: sv.TrustedAt,
		ConfigFile: sv.ConfigFile, Node: sv.Node, Relays: sv.Relays}
	if sv.ConfigFile != "" {
		path := filepath.Join(sv.ConfigDir, sv.ConfigFile)
		if _, err := os.Stat(path); err == nil {
			dto.ConfigPath = path
			dto.InLauncher = launcherFound && strings.EqualFold(filepath.Clean(sv.ConfigDir), filepath.Clean(configsDir))
		}
	}
	return dto
}

// ServerInput is what the add and edit dialogs send.
type ServerInput struct {
	Name        string `json:"name"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	AuthMethod  string `json:"authMethod"`
	KeyPath     string `json:"keyPath"`
	Fingerprint string `json:"fingerprint"`
	Algorithm   string `json:"algorithm"`
}

// TaskOptions are StartTask's parameters for relays; see tasks.Options.
type TaskOptions struct {
	ConfigName      string   `json:"configName"` // a file in the configurations folder
	FrontID         string   `json:"frontId"`
	FrontPassword   string   `json:"frontPassword"`
	FrontPassphrase string   `json:"frontPassphrase"`
	Remark          string   `json:"remark"`
	RateMbps        int      `json:"rateMbps"`
	Target          string   `json:"target"`
	Action          string   `json:"action"`
	Items           []string `json:"items"`
}

// ConfigDTO is a node configuration in the configurations folder.
type ConfigDTO struct {
	Name  string `json:"name"`
	Entry string `json:"entry"` // host:port it connects to
}

// FriendCheckDTO is CheckFriendEntry's answer.
type FriendCheckDTO struct {
	Entry     string `json:"entry"`
	Target    string `json:"target"`
	Node      string `json:"node"`
	Matches   string `json:"matches"`   // yes, no or unknown
	Reachable string `json:"reachable"` // from this computer
}

// ProbeDTO is a server's host key.
type ProbeDTO struct {
	Fingerprint string `json:"fingerprint"`
	Algorithm   string `json:"algorithm"`
}

// ComponentDTO is one open-source component on the license dialog.
type ComponentDTO struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Copyright string `json:"copyright"`
	License   string `json:"license"`
	URL       string `json:"url"`
}

// LinkDTO is a named link.
type LinkDTO struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// AboutDTO backs the about card and the license dialog.
type AboutDTO struct {
	Version      string         `json:"version"`
	Copyright    string         `json:"copyright"`
	License      string         `json:"license"`
	Scripts      string         `json:"scripts"` // where the bundled installer comes from
	Components   []ComponentDTO `json:"components"`
	ServerSide   []ComponentDTO `json:"serverSide"` // installed on servers, not shipped
	LicenseTexts []LinkDTO      `json:"licenseTexts"`
}
