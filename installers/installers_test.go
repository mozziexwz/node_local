package installers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// The synced files must stay byte-identical to what UPSTREAM.json records;
// a hand edit or a CRLF checkout would change what servers run.
func TestSyncedFilesMatchUpstream(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "UPSTREAM.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Commit string
		Files  []struct{ Path, SHA256 string }
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(m.Commit) || len(m.Files) == 0 {
		t.Fatalf("UPSTREAM.json is incomplete: %s", raw)
	}
	recorded := map[string]string{}
	for _, f := range m.Files {
		data, err := os.ReadFile(filepath.Join("..", filepath.FromSlash(f.Path)))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != f.SHA256 {
			t.Errorf("%s differs from the synced copy; resync instead of editing it", f.Path)
		}
		recorded[f.Path] = f.SHA256
	}
	if NodeScript().SHA256 != recorded["installers/node/msboost.sh"] {
		t.Error("embedded installer is not the synced one")
	}
}

func TestEngineHasGostPins(t *testing.T) {
	e := NewEngine()
	url := regexp.MustCompile(`^https://github\.com/go-gost/gost/releases/download/v`)
	sum := regexp.MustCompile(`^[0-9a-f]{64}$`)
	if !url.MatchString(e.GostAMD64URL) || !url.MatchString(e.GostARM64URL) || !sum.MatchString(e.GostAMD64SHA256) || !sum.MatchString(e.GostARM64SHA256) {
		t.Fatalf("GOST pins missing: %+v", e)
	}
}
