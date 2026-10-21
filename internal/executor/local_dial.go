package executor

import (
	"context"
	"net"
)

// Local edition: the app chooses how SSH connections are opened, for example
// through a local SOCKS5 proxy when direct SSH to the VPS is unreliable.

// NewSSHRemote returns an SSHRemote whose TCP connections are opened by dial.
// Run and Probe set no dial timeout of their own on this path, so dial must
// give up by itself.
func NewSSHRemote(dial func(ctx context.Context, network, address string) (net.Conn, error)) SSHRemote {
	return SSHRemote{dialContext: dial}
}
