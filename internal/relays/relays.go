// Package relays remembers what the relays built by this program are for, in
// local\relays.json. The relay servers keep no such record: the website only
// accepts records with an account e-mail (see the plan, §4.6), so the
// remark, purpose and target of a relay live here. Losing the file loses
// only these labels; the relays keep working.
package relays

import (
	"encoding/json"
	"errors"
	"os"
	"sort"
	"sync"

	"github.com/mozziexwz/node_local/internal/store"
)

// Kinds of relay.
const (
	Self   = "self"   // to one of the user's own nodes
	Friend = "friend" // to a friend's node, by its address
	Front  = "front"  // a front server before one of the user's relays
)

// Label describes one managed relay on a server.
type Label struct {
	ID         string `json:"id"` // the managed relay ID on the server, e.g. "3f2a91c0d4e5b6a7r"
	ServerID   string `json:"serverId"`
	Kind       string `json:"kind"`
	Remark     string `json:"remark"`
	Target     string `json:"target"` // host:port it forwards to
	Entry      string `json:"entry"`  // host:port players connect to
	ConfigFile string `json:"configFile,omitempty"`
	CreatedAt  int64  `json:"createdAt"`
}

type file struct {
	Version int     `json:"version"`
	Relays  []Label `json:"relays"`
}

// Store is safe for concurrent use.
type Store struct {
	path string
	mu   sync.Mutex
	list []Label
}

// Open reads the labels. A file that cannot be read is reported and replaced
// on the next change.
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
		return s, errors.New("中转记录文件 local\\relays.json 无法读取，中转列表里的备注暂时不会显示")
	}
	s.list = f.Relays
	return s, nil
}

// Add records new relays.
func (s *Store) Add(labels ...Label) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := append([]Label(nil), s.list...)
	for _, l := range labels {
		list = append(removeID(list, l.ServerID, l.ID), l)
	}
	return s.saveLocked(list)
}

// Remove forgets relays on one server.
func (s *Store) Remove(serverID string, ids ...string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := append([]Label(nil), s.list...)
	for _, id := range ids {
		list = removeID(list, serverID, id)
	}
	if len(list) == len(s.list) {
		return nil
	}
	return s.saveLocked(list)
}

// RemoveServer forgets every relay on one server.
func (s *Store) RemoveServer(serverID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := make([]Label, 0, len(s.list))
	for _, l := range s.list {
		if l.ServerID != serverID {
			list = append(list, l)
		}
	}
	if len(list) == len(s.list) {
		return nil
	}
	return s.saveLocked(list)
}

// Find returns the label of one relay.
func (s *Store) Find(serverID, id string) (Label, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, l := range s.list {
		if l.ServerID == serverID && l.ID == id {
			return l, true
		}
	}
	return Label{}, false
}

// ForServer lists one server's labels, oldest first.
func (s *Store) ForServer(serverID string) []Label {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Label
	for _, l := range s.list {
		if l.ServerID == serverID {
			out = append(out, l)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt < out[j].CreatedAt })
	return out
}

func removeID(list []Label, serverID, id string) []Label {
	out := list[:0:0]
	for _, l := range list {
		if l.ServerID != serverID || l.ID != id {
			out = append(out, l)
		}
	}
	return out
}

func (s *Store) saveLocked(list []Label) error {
	if err := store.WriteJSON(s.path, file{Version: 1, Relays: list}); err != nil {
		return err
	}
	s.list = list
	return nil
}
