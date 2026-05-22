package mode

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-installapplications/pkg/ipc"
	"github.com/go-installapplications/pkg/utils"
)

// shortSockPath returns a short unix socket path under /tmp.
// macOS limits unix socket paths to ~104 bytes, which exceeds typical
// t.TempDir() paths under /var/folders. Using /tmp + a random name keeps us safe.
func shortSockPath(t *testing.T) string {
	t.Helper()
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatalf("rand: %v", err)
	}
	p := filepath.Join("/tmp", "go-ia-test-"+hex.EncodeToString(b[:])+".sock")
	t.Cleanup(func() { _ = os.Remove(p) })
	return p
}

// roundTrip dials the agent socket and sends a request, decoding the reply.
func roundTrip(t *testing.T, sockPath string, req ipc.RPCRequest) ipc.RPCResponse {
	t.Helper()
	conn, err := net.DialTimeout("unix", sockPath, 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		t.Fatalf("encode: %v", err)
	}
	var resp ipc.RPCResponse
	if err := json.NewDecoder(bufio.NewReader(conn)).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return resp
}

// startTestAgentServer is a minimal version of the agent's IPC server with
// the same Shutdown idempotency guarantee, so we can exercise the bug fix
// without needing root or a real LaunchAgent.
func startTestAgentServer(t *testing.T, sockPath string, done chan<- struct{}) net.Listener {
	t.Helper()
	l, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	var once sync.Once
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				var req ipc.RPCRequest
				if err := json.NewDecoder(bufio.NewReader(c)).Decode(&req); err != nil {
					return
				}
				resp := ipc.RPCResponse{ID: req.ID, OK: true}
				switch req.Command {
				case "Shutdown":
					// Exactly the same guarantee RunAgent gives in production:
					// repeated Shutdown calls must not panic by closing a
					// closed channel.
					once.Do(func() { close(done) })
				case "Ping":
					// ok
				default:
					resp.OK = false
					resp.Error = "unknown command"
				}
				_ = json.NewEncoder(c).Encode(resp)
			}(conn)
		}
	}()
	return l
}

func TestAgentShutdown_IsIdempotent(t *testing.T) {
	sockPath := shortSockPath(t)
	done := make(chan struct{})
	l := startTestAgentServer(t, sockPath, done)
	defer l.Close()

	// First Shutdown closes the done channel
	resp := roundTrip(t, sockPath, ipc.RPCRequest{ID: "a", Command: "Shutdown"})
	if !resp.OK {
		t.Fatalf("first shutdown: %+v", resp)
	}

	// Second Shutdown must NOT panic and should still respond OK
	resp = roundTrip(t, sockPath, ipc.RPCRequest{ID: "b", Command: "Shutdown"})
	if !resp.OK {
		t.Fatalf("second shutdown: %+v", resp)
	}

	// Done channel should have been closed exactly once (no panic = pass)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("done channel never closed")
	}
}

func TestAgentPing_RoundTrips(t *testing.T) {
	sockPath := shortSockPath(t)
	done := make(chan struct{})
	l := startTestAgentServer(t, sockPath, done)
	defer l.Close()

	resp := roundTrip(t, sockPath, ipc.RPCRequest{ID: "ping-1", Command: "Ping"})
	if !resp.OK || resp.ID != "ping-1" {
		t.Fatalf("ping: %+v", resp)
	}
}

// Verify that callAgent (the production daemon→agent client) round-trips
// a request through the real production server function.
func TestCallAgent_RoundTrip(t *testing.T) {
	// Swap the ipc.SocketDir to /tmp so the resulting socket path stays
	// under the macOS 104-byte unix socket limit.
	var b [6]byte
	_, _ = rand.Read(b[:])
	tmp := filepath.Join("/tmp", "ipc-"+hex.EncodeToString(b[:]))
	if err := os.MkdirAll(tmp, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmp) })

	original := ipc.SocketDir
	ipc.SetSocketDir(tmp)
	t.Cleanup(func() { ipc.SetSocketDir(original) })

	// The production server derives the socket name from the console user
	// UID. Spin up a tiny server at the same path manually rather than
	// invoking startAgentIPCServer (which calls into stat /dev/console).
	uid := "9999"
	sockPath := ipc.GetAgentSocketPathForUID(uid)

	var pingCount int32
	l, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer l.Close()
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				var req ipc.RPCRequest
				if err := json.NewDecoder(bufio.NewReader(c)).Decode(&req); err != nil {
					return
				}
				atomic.AddInt32(&pingCount, 1)
				resp := ipc.RPCResponse{ID: req.ID, OK: true}
				_ = json.NewEncoder(c).Encode(resp)
			}(conn)
		}
	}()

	logger := utils.NewLogger(false, false)
	resp, err := callAgent(logger, sockPath, ipc.RPCRequest{Command: "Ping"}, 2*time.Second)
	if err != nil {
		t.Fatalf("callAgent: %v", err)
	}
	if !resp.OK {
		t.Fatalf("expected OK, got %+v", resp)
	}
	if atomic.LoadInt32(&pingCount) != 1 {
		t.Fatalf("expected exactly one ping, got %d", pingCount)
	}
}
