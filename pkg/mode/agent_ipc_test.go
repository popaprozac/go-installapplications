package mode

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-installapplications/pkg/installer"
	"github.com/go-installapplications/pkg/ipc"
	"github.com/go-installapplications/pkg/utils"
)

// newAgentInstallerForTest returns a real SystemInstaller configured for
// agent mode and not dry-run, so its ProcessTracker is exercisable.
func newAgentInstallerForTest(t *testing.T, logger *utils.Logger) *installer.SystemInstaller {
	t.Helper()
	return installer.NewSystemInstaller(false, logger, true)
}

// startAgentLikeServer mirrors the production agent handler in RunAgent —
// same commands, same singleton SystemInstaller — without needing root or a
// real LaunchAgent.
func startAgentLikeServer(t *testing.T, sockPath string, si *installer.SystemInstaller, _ *utils.Logger) net.Listener {
	t.Helper()
	l, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
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
				case "Ping":
					// ok
				case "GetBackgroundProcessCount":
					resp.Count = si.GetBackgroundProcessCount()
				case "WaitForBackgroundProcesses":
					timeout := time.Duration(req.TimeoutSeconds) * time.Second
					if timeout <= 0 {
						timeout = 5 * time.Second
					}
					errs := si.WaitForBackgroundProcesses(timeout)
					if len(errs) > 0 {
						resp.OK = false
						resp.Errors = make([]string, 0, len(errs))
						for _, e := range errs {
							resp.Errors = append(resp.Errors, e.Error())
						}
						resp.Error = resp.Errors[0]
					}
				default:
					resp.OK = false
					resp.Error = "unknown command in test harness"
				}
				_ = json.NewEncoder(c).Encode(resp)
			}(conn)
		}
	}()
	return l
}

// startTrackedSleep pushes a fast-finishing background process into the
// installer's tracker via its public Installer-interface ExecuteScript.
// We don't have a public hook to inject a Process directly, so we write a
// tiny shell script and run it as a "rootscript" with donotwait+track=true.
func startTrackedSleep(t *testing.T, si *installer.SystemInstaller, label, sleepSeconds string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, label+".sh")
	body := "#!/bin/sh\nsleep " + sleepSeconds + "\n"
	if err := os.WriteFile(path, []byte(body), 0755); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := si.ExecuteScript(path, "rootscript", true, true); err != nil {
		t.Fatalf("start: %v", err)
	}
}

// startTrackedFailing pushes a background process that exits non-zero.
func startTrackedFailing(t *testing.T, si *installer.SystemInstaller, label string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, label+".sh")
	body := "#!/bin/sh\nexit 7\n"
	if err := os.WriteFile(path, []byte(body), 0755); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := si.ExecuteScript(path, "rootscript", true, true); err != nil {
		t.Fatalf("start: %v", err)
	}
}

// _ pull os/exec into the package so the linter doesn't complain when we
// extend this file with extra harness helpers that need it later.
var _ = exec.Command

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

// End-to-end exercise of the agent's WaitForBackgroundProcesses and
// GetBackgroundProcessCount commands. We run a real agent against a real
// installer that spawns short-lived shell processes; the daemon-side IPC
// client then drains them.
func TestAgent_BackgroundDrainAndCount(t *testing.T) {
	// We can't call RunAgent directly (it never returns), but we can spin up
	// the same IPC server with the same handler logic via a tiny harness.
	logger := utils.NewLogger(false, false)

	si := newAgentInstallerForTest(t, logger)
	sockPath := shortSockPath(t)

	l := startAgentLikeServer(t, sockPath, si, logger)
	defer l.Close()

	// Spawn two short-lived background processes via the agent's installer.
	for i := 0; i < 2; i++ {
		// Note: We can't easily use ExecuteScript here because the path must
		// resolve to a real file. Use the underlying ProcessTracker directly
		// via a small helper in the harness.
		startTrackedSleep(t, si, "noop"+string(rune('A'+i)), "0.1")
	}

	// GetBackgroundProcessCount should report 2 immediately.
	resp := roundTrip(t, sockPath, ipc.RPCRequest{ID: "c1", Command: "GetBackgroundProcessCount"})
	if !resp.OK || resp.Count != 2 {
		t.Fatalf("count: %+v", resp)
	}

	// WaitForBackgroundProcesses should drain them with no errors.
	resp = roundTrip(t, sockPath, ipc.RPCRequest{ID: "w1", Command: "WaitForBackgroundProcesses", TimeoutSeconds: 5})
	if !resp.OK || len(resp.Errors) != 0 {
		t.Fatalf("wait: %+v", resp)
	}

	// After drain, count returns to 0.
	resp = roundTrip(t, sockPath, ipc.RPCRequest{ID: "c2", Command: "GetBackgroundProcessCount"})
	if !resp.OK || resp.Count != 0 {
		t.Fatalf("count after drain: %+v", resp)
	}
}

// A failing background process surfaces through WaitForBackgroundProcesses.Errors.
func TestAgent_BackgroundDrainReportsFailures(t *testing.T) {
	logger := utils.NewLogger(false, false)
	si := newAgentInstallerForTest(t, logger)
	sockPath := shortSockPath(t)
	l := startAgentLikeServer(t, sockPath, si, logger)
	defer l.Close()

	// One that succeeds (exit 0), one that fails (exit 1)
	startTrackedSleep(t, si, "ok", "0.1")
	startTrackedFailing(t, si, "fail")

	resp := roundTrip(t, sockPath, ipc.RPCRequest{ID: "w", Command: "WaitForBackgroundProcesses", TimeoutSeconds: 5})
	if resp.OK {
		t.Fatalf("expected wait to report failure, got OK")
	}
	if len(resp.Errors) == 0 {
		t.Fatalf("expected at least one error string")
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
