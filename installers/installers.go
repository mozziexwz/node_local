// Package installers embeds what the executor sends to servers: the node
// installer and the pinned GOST archives for relays. Both are synced from the
// website repository (see UPSTREAM.json); never edit them here, run
// `go run ./scripts/syncnode -tag <tag>` instead.
package installers

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"

	"github.com/mozziexwz/node_local/internal/executor"
)

//go:embed node/msboost.sh
var nodeScript []byte

//go:embed gost.json
var gostJSON []byte

// NodeScript returns the node installer as the executor's deploy task takes it.
func NodeScript() executor.Asset {
	sum := sha256.Sum256(nodeScript)
	return executor.Asset{Data: nodeScript, SHA256: hex.EncodeToString(sum[:])}
}

type gostPin struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
}

// NewEngine returns an executor engine with the pinned GOST archives, which
// relay tasks download onto the relay server and check before use.
func NewEngine() *executor.Engine {
	var pins map[string]gostPin
	if err := json.Unmarshal(gostJSON, &pins); err != nil {
		panic("installers: gost.json: " + err.Error())
	}
	e := executor.NewEngine()
	e.GostAMD64URL, e.GostAMD64SHA256 = pins["amd64"].URL, pins["amd64"].SHA256
	e.GostARM64URL, e.GostARM64SHA256 = pins["arm64"].URL, pins["arm64"].SHA256
	return e
}
