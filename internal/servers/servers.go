// Package servers keeps the saved servers in local\servers.json: address,
// login method, private key location and the pinned host key. Passwords and
// key passphrases are never stored.
package servers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mozziexwz/node_local/internal/executor"
	"github.com/mozziexwz/node_local/internal/store"
)

// Server is one saved server. The SSH user is always root, as the node
// installer requires.
type Server struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	AuthMethod  string `json:"authMethod"`        // "password" or "key"
	KeyPath     string `json:"keyPath,omitempty"` // OpenSSH private key file for "key"
	Fingerprint string `json:"fingerprint"`
	Algorithm   string `json:"algorithm"`
	TrustedAt   int64  `json:"trustedAt"` // Unix ms when the host key was trusted
	// ConfigFile in ConfigDir is the configuration last written for this
	// server's node.
	ConfigFile string     `json:"configFile,omitempty"`
	ConfigDir  string     `json:"configDir,omitempty"`
	Node       *NodeState `json:"node,omitempty"`
	CreatedAt  int64      `json:"createdAt"`
}

// NodeState is what the last task or status check found on the server.
type NodeState struct {
	Installed bool   `json:"installed"`
	Service   string `json:"service,omitempty"` // running or stopped
	Engine    string `json:"engine,omitempty"`
	BBR       string `json:"bbr,omitempty"`
	OS        string `json:"os,omitempty"`
	Entry     string `json:"entry,omitempty"`     // the node's address from its configuration
	PublicTCP string `json:"publicTCP,omitempty"` // from this computer
	CheckedAt int64  `json:"checkedAt"`
}

// Label is the name shown for the server: its remark, or its address.
func (s Server) Label() string {
	if s.Name != "" {
		return s.Name
	}
	return s.Host
}

// Address is host:port.
func (s Server) Address() string { return net.JoinHostPort(s.Host, strconv.Itoa(s.Port)) }

// ErrBroken protects a servers.json that could not be read from being
// overwritten with an empty list.
var ErrBroken = errors.New("服务器列表文件 local\\servers.json 无法读取。为免覆盖，暂时不能修改服务器；请修复或删除这个文件后重新打开程序")

var fingerprintPattern = regexp.MustCompile(`^SHA256:[A-Za-z0-9+/]{43}$`)

// Validate checks a server before it is saved.
func Validate(s Server) error {
	if len([]rune(s.Name)) > 30 || strings.ContainsAny(s.Name, "\x00\r\n\t") {
		return errors.New("备注最多 30 个字，不能换行")
	}
	if err := executor.PublicIP(s.Host); err != nil {
		return err
	}
	if s.Port < 1 || s.Port > 65535 {
		return errors.New("SSH 端口必须为 1–65535")
	}
	switch s.AuthMethod {
	case "password":
		if s.KeyPath != "" {
			return errors.New("密码登录不需要私钥文件")
		}
	case "key":
		if !filepath.IsAbs(s.KeyPath) {
			return errors.New("请选择私钥文件")
		}
	default:
		return errors.New("登录方式无效")
	}
	if !fingerprintPattern.MatchString(s.Fingerprint) {
		return errors.New("请先读取并确认服务器指纹")
	}
	return nil
}

type file struct {
	Version int      `json:"version"`
	Servers []Server `json:"servers"`
}

// Store is the server list. It is safe for concurrent use.
type Store struct {
	path   string
	mu     sync.Mutex
	list   []Server
	broken bool
}

// Open reads the list. A missing file is an empty list. A file that cannot be
// read gives an empty, read-only store and ErrBroken.
func Open(path string) (*Store, error) {
	s := &Store{path: path}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	var f file
	if err == nil {
		err = json.Unmarshal(raw, &f)
	}
	if err != nil {
		s.broken = true
		return s, ErrBroken
	}
	s.list = f.Servers
	return s, nil
}

// List returns the servers in the order they were added.
func (s *Store) List() []Server {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := append([]Server(nil), s.list...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt < out[j].CreatedAt })
	return out
}

// Get returns one server.
func (s *Store) Get(id string) (Server, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, v := range s.list {
		if v.ID == id {
			return v, true
		}
	}
	return Server{}, false
}

// Add saves a new server with a fresh ID.
func (s *Store) Add(v Server) (Server, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.broken {
		return Server{}, ErrBroken
	}
	if err := Validate(v); err != nil {
		return Server{}, err
	}
	for _, other := range s.list {
		if other.Address() == v.Address() {
			return Server{}, fmt.Errorf("%s 已经在列表里（%s）", v.Address(), other.Label())
		}
	}
	v.ID = newID()
	now := time.Now().UnixMilli()
	v.CreatedAt, v.TrustedAt = now, now
	v.ConfigFile, v.ConfigDir, v.Node = "", "", nil
	if err := s.saveLocked(append(append([]Server(nil), s.list...), v)); err != nil {
		return Server{}, err
	}
	return v, nil
}

// Update changes one server through change and saves the list. A change that
// fails validation leaves everything as it was.
func (s *Store) Update(id string, change func(*Server) error) (Server, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.broken {
		return Server{}, ErrBroken
	}
	list := append([]Server(nil), s.list...)
	for i := range list {
		if list[i].ID != id {
			continue
		}
		v := list[i]
		if err := change(&v); err != nil {
			return Server{}, err
		}
		v.ID, v.CreatedAt = list[i].ID, list[i].CreatedAt
		if err := Validate(v); err != nil {
			return Server{}, err
		}
		for _, other := range list {
			if other.ID != id && other.Address() == v.Address() {
				return Server{}, fmt.Errorf("%s 已经在列表里（%s）", v.Address(), other.Label())
			}
		}
		list[i] = v
		if err := s.saveLocked(list); err != nil {
			return Server{}, err
		}
		return v, nil
	}
	return Server{}, errors.New("找不到这台服务器")
}

// Remove deletes a server from the list. Nothing on the server changes.
func (s *Store) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.broken {
		return ErrBroken
	}
	list := make([]Server, 0, len(s.list))
	for _, v := range s.list {
		if v.ID != id {
			list = append(list, v)
		}
	}
	if len(list) == len(s.list) {
		return errors.New("找不到这台服务器")
	}
	return s.saveLocked(list)
}

func (s *Store) saveLocked(list []Server) error {
	if err := store.WriteJSON(s.path, file{Version: 1, Servers: list}); err != nil {
		return fmt.Errorf("无法保存服务器列表：%w", err)
	}
	s.list = list
	return nil
}

func newID() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
