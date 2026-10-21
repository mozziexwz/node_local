package servers

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testFingerprint = "SHA256:Kd3pQ9vX2mT8nL4rY7cB1fH6jW0sE5uA3zG9kP2qV6o"

func testServer() Server {
	return Server{Name: "东京节点", Host: "8.8.8.8", Port: 22, AuthMethod: "password", Fingerprint: testFingerprint, Algorithm: "ecdsa-sha2-nistp256"}
}

func TestAddUpdateRemovePersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "servers.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	added, err := s.Add(testServer())
	if err != nil {
		t.Fatal(err)
	}
	if added.ID == "" || added.CreatedAt == 0 || added.TrustedAt == 0 {
		t.Fatalf("added %+v", added)
	}
	if _, err := s.Add(testServer()); err == nil || !strings.Contains(err.Error(), "已经在列表里") {
		t.Fatalf("duplicate address: %v", err)
	}
	if _, err := s.Update(added.ID, func(v *Server) error { v.Name = "东京"; v.ConfigFile = "东京.json"; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update(added.ID, func(v *Server) error { v.Port = 0; return nil }); err == nil {
		t.Fatal("accepted an invalid port")
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := reopened.Get(added.ID)
	if !ok || got.Name != "东京" || got.ConfigFile != "东京.json" || got.Port != 22 {
		t.Fatalf("reopened %+v", got)
	}
	raw, _ := os.ReadFile(path)
	for _, secret := range []string{"password\":", "passphrase"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("servers.json must not hold %s: %s", secret, raw)
		}
	}
	if err := reopened.Remove(added.ID); err != nil {
		t.Fatal(err)
	}
	if len(reopened.List()) != 0 {
		t.Fatal("not removed")
	}
}

func TestValidate(t *testing.T) {
	for name, change := range map[string]func(*Server){
		"private address": func(v *Server) { v.Host = "192.168.1.2" },
		"bad port":        func(v *Server) { v.Port = 70000 },
		"long remark":     func(v *Server) { v.Name = strings.Repeat("节", 31) },
		"newline":         func(v *Server) { v.Name = "a\nb" },
		"no fingerprint":  func(v *Server) { v.Fingerprint = "" },
		"key without file": func(v *Server) {
			v.AuthMethod = "key"
		},
		"password with key file": func(v *Server) { v.KeyPath = `C:\id_ed25519` },
		"unknown method":         func(v *Server) { v.AuthMethod = "kerberos" },
	} {
		v := testServer()
		change(&v)
		if Validate(v) == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	v := testServer()
	v.AuthMethod, v.KeyPath = "key", filepath.Join(t.TempDir(), "id_ed25519")
	if err := Validate(v); err != nil {
		t.Fatalf("key login: %v", err)
	}
}

func TestBrokenFileIsNeverOverwritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "servers.json")
	os.WriteFile(path, []byte("{not json"), 0o644)
	s, err := Open(path)
	if !errors.Is(err, ErrBroken) {
		t.Fatalf("open: %v", err)
	}
	if _, err := s.Add(testServer()); !errors.Is(err, ErrBroken) {
		t.Fatalf("add: %v", err)
	}
	if raw, _ := os.ReadFile(path); string(raw) != "{not json" {
		t.Fatal("the broken file was overwritten")
	}
}
