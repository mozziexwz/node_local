package executor

import (
	"bytes"
	"context"
	"net"
	"os/exec"
	"runtime"
	"testing"
)

// bashRemote runs scripts on this machine, as a relay server would.
type bashRemote struct{}

func (bashRemote) Run(ctx context.Context, _ SSH, script string) ([]byte, error) {
	return exec.CommandContext(ctx, "bash", "-c", script).CombinedOutput()
}

func (bashRemote) Probe(context.Context, string, int) (string, string, error) { return "", "", nil }

// udpServer reads every datagram on a loopback port and answers those that
// answer says to.
func udpServer(t *testing.T, answer func([]byte) bool) int {
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	go func() {
		buf := make([]byte, 65535)
		for {
			n, from, err := conn.ReadFrom(buf)
			if err != nil {
				return
			}
			if answer(buf[:n]) {
				conn.WriteTo([]byte("ok"), from)
			}
		}
	}()
	return conn.LocalAddr().(*net.UDPAddr).Port
}

func TestUDPTargetCheck(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the check runs on Debian relay servers")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is not installed")
	}
	e := &Engine{Remote: bashRemote{}}
	check := func(port int) string { return e.checkUDPTarget(context.Background(), SSH{}, "127.0.0.1", port) }

	// Like a Mieru port: it reads everything and answers nothing.
	if v := check(udpServer(t, func([]byte) bool { return false })); v != "silent" {
		t.Errorf("a silent port: %s", v)
	}
	// A service that answers only its own protocol is still caught.
	if v := check(udpServer(t, func(b []byte) bool { return bytes.HasPrefix(b, []byte("M-SEARCH")) })); v != "answered" {
		t.Errorf("an SSDP responder: %s", v)
	}
	closed, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := closed.LocalAddr().(*net.UDPAddr).Port
	closed.Close()
	if v := check(port); v != "refused" {
		t.Errorf("a closed port: %s", v)
	}
}
