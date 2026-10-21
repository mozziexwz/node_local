package executor

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// Local edition: Run with live output, so the app can show the installer's
// progress while it runs.

// StreamRemote runs a script like Remote.Run and also passes each complete
// output line to onLine as it arrives.
type StreamRemote interface {
	RunStream(ctx context.Context, s SSH, script string, onLine func(string)) ([]byte, error)
}

// RunStream is SSHRemote.Run (ssh.go) with stdout also split into lines for
// onLine. Keep the connection handling identical to Run when syncing ssh.go.
func (remote SSHRemote) RunStream(ctx context.Context, s SSH, script string, onLine func(string)) ([]byte, error) {
	if err := validateSSHFields(s); err != nil {
		return nil, err
	}
	auth, err := sshAuthMethods(s)
	if err != nil {
		return nil, err
	}
	address := net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
	conn, err := remote.dial(ctx, address, 15*time.Second)
	if err != nil {
		return nil, connectDiagnostic(err)
	}
	defer conn.Close()
	stopClose := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stopClose()
	_ = conn.SetDeadline(time.Now().Add(20 * time.Second))
	hostChanged := false
	pin := PinnedHostKey(s.Fingerprint)
	clientConn, chans, reqs, err := ssh.NewClientConn(conn, address, &ssh.ClientConfig{User: s.User, Auth: auth, HostKeyCallback: func(host string, addr net.Addr, key ssh.PublicKey) error {
		err := pin(host, addr, key)
		hostChanged = err != nil
		return err
	}, Timeout: 15 * time.Second})
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if hostChanged {
			return nil, diagnosticError("ssh_host_changed")
		}
		if strings.Contains(err.Error(), "unable to authenticate") {
			return nil, diagnosticError("ssh_auth")
		}
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return nil, diagnosticError("ssh_timeout")
		}
		return nil, diagnosticError("ssh_handshake")
	}
	_ = conn.SetDeadline(time.Time{})
	client := ssh.NewClient(clientConn, chans, reqs)
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, diagnosticError("ssh_session")
	}
	defer session.Close()
	var output limitedOutput
	lines := &lineWriter{out: &output, fn: onLine}
	session.Stdout = lines
	session.Stderr = &output
	session.Stdin = bytes.NewBufferString(script)
	done := make(chan error, 1)
	go func() { done <- session.Run("bash -s") }()
	select {
	case err = <-done:
		lines.flush()
		if ctx.Err() != nil {
			return output.Bytes(), ctx.Err()
		}
		return output.Bytes(), err
	case <-ctx.Done():
		_ = client.Close()
		<-done
		lines.flush()
		return output.Bytes(), ctx.Err()
	}
}

// lineWriter keeps all output in out (which bounds it) and hands complete
// lines to fn, outside its lock.
type lineWriter struct {
	out *limitedOutput
	fn  func(string)

	mu      sync.Mutex
	partial []byte
}

const maxPartialLine = 64 * 1024

func (w *lineWriter) Write(p []byte) (int, error) {
	n, err := w.out.Write(p)
	w.mu.Lock()
	w.partial = append(w.partial, p...)
	var lines []string
	for {
		i := bytes.IndexByte(w.partial, '\n')
		if i < 0 {
			break
		}
		lines = append(lines, strings.TrimRight(string(w.partial[:i]), "\r"))
		w.partial = w.partial[i+1:]
	}
	if len(w.partial) > maxPartialLine {
		w.partial = nil // no marker line is this long
	}
	w.mu.Unlock()
	for _, line := range lines {
		w.fn(line)
	}
	return n, err
}

// flush passes on a last line that ended without a newline.
func (w *lineWriter) flush() {
	w.mu.Lock()
	rest := strings.TrimRight(string(w.partial), "\r")
	w.partial = nil
	w.mu.Unlock()
	if rest != "" {
		w.fn(rest)
	}
}

var _ io.Writer = (*lineWriter)(nil)
