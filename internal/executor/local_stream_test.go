package executor

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// streamServer is an in-process SSH server that accepts password "pw",
// reads the whole script and then writes output in pieces, pausing between
// them, before exiting with status code.
func streamServer(t *testing.T, pieces []string, code uint32) (string, string) {
	t.Helper()
	_, hostKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, _ := ssh.NewSignerFromKey(hostKey)
	config := &ssh.ServerConfig{PasswordCallback: func(_ ssh.ConnMetadata, given []byte) (*ssh.Permissions, error) {
		if string(given) == "pw" {
			return nil, nil
		}
		return nil, errors.New("wrong password")
	}}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				server, channels, requests, err := ssh.NewServerConn(conn, config)
				if err != nil {
					return
				}
				defer server.Close()
				go ssh.DiscardRequests(requests)
				for incoming := range channels {
					channel, channelRequests, err := incoming.Accept()
					if err != nil {
						return
					}
					for request := range channelRequests {
						if request.Type != "exec" {
							_ = request.Reply(false, nil)
							continue
						}
						_ = request.Reply(true, nil)
						_, _ = io.Copy(io.Discard, channel)
						for _, p := range pieces {
							_, _ = channel.Write([]byte(p))
							time.Sleep(20 * time.Millisecond)
						}
						status := make([]byte, 4)
						binary.BigEndian.PutUint32(status, code)
						_, _ = channel.SendRequest("exit-status", false, status)
						_ = channel.Close()
						break
					}
				}
			}()
		}
	}()
	return listener.Addr().String(), ssh.FingerprintSHA256(signer.PublicKey())
}

// streamTarget dials the test server whatever public address SSH names.
func streamTarget(address, fingerprint string) (SSHRemote, SSH) {
	remote := NewSSHRemote(func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, address)
	})
	s := localTestSSH()
	s.Password, s.Fingerprint = "pw", fingerprint
	return remote, s
}

func TestRunStreamReportsLinesAsTheyArrive(t *testing.T) {
	pieces := []string{"MSBOOST_STEP=Installing ", "required packages\r\nMSBOOST_", "STEP=Deploying msboost\nMSBOOST_CONFIG=e30=\n", "no newline"}
	address, fingerprint := streamServer(t, pieces, 0)
	remote, s := streamTarget(address, fingerprint)
	var mu sync.Mutex
	var lines []string
	out, err := remote.RunStream(context.Background(), s, "echo\n", func(line string) {
		mu.Lock()
		lines = append(lines, line)
		mu.Unlock()
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"MSBOOST_STEP=Installing required packages", "MSBOOST_STEP=Deploying msboost", "MSBOOST_CONFIG=e30=", "no newline"}
	if strings.Join(lines, "|") != strings.Join(want, "|") {
		t.Fatalf("lines %q, want %q", lines, want)
	}
	if string(out) != strings.Join(pieces, "") {
		t.Fatalf("output %q", out)
	}
	// Run sees the same output from the same server.
	plain, err := remote.Run(context.Background(), s, "echo\n")
	if err != nil || string(plain) != string(out) {
		t.Fatalf("Run returned %q, %v", plain, err)
	}
}

func TestRunStreamKeepsTheSecurityChecks(t *testing.T) {
	address, fingerprint := streamServer(t, []string{"x\n"}, 0)
	remote, s := streamTarget(address, fingerprint)
	never := func(string) { t.Error("no output may arrive") }

	wrongKey := s
	wrongKey.Fingerprint = "SHA256:" + strings.Repeat("B", 43)
	if _, err := remote.RunStream(context.Background(), wrongKey, "echo\n", never); !isDiagnostic(err, "ssh_host_changed") {
		t.Fatalf("changed host key: %v", err)
	}
	wrongPassword := s
	wrongPassword.Password = "nope"
	if _, err := remote.RunStream(context.Background(), wrongPassword, "echo\n", never); !isDiagnostic(err, "ssh_auth") {
		t.Fatalf("wrong password: %v", err)
	}
	private := s
	private.Host = "10.0.0.1"
	if _, err := remote.RunStream(context.Background(), private, "echo\n", never); err == nil {
		t.Fatal("accepted a private address")
	}
}

func TestRunStreamReportsExitStatus(t *testing.T) {
	address, fingerprint := streamServer(t, []string{"MSBOOST_ERROR_CODE=download_failed\nMSBOOST_ERROR_PHASE=download\n"}, 1)
	remote, s := streamTarget(address, fingerprint)
	out, err := remote.RunStream(context.Background(), s, "echo\n", func(string) {})
	var exit *ssh.ExitError
	if !errors.As(err, &exit) || exit.ExitStatus() != 1 {
		t.Fatalf("exit status: %v", err)
	}
	if d := remoteDiagnostic(err, out, "install"); d.Code != "download_failed" {
		t.Fatalf("diagnostic %+v", d)
	}
}

func isDiagnostic(err error, code string) bool {
	var d Diagnostic
	return errors.As(err, &d) && d.Code == code
}
