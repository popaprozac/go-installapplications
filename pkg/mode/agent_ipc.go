package mode

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"github.com/go-installapplications/pkg/ipc"
	"github.com/go-installapplications/pkg/utils"
)

// startAgentIPCServer starts a Unix domain socket server to handle user-context requests from the daemon.
// The agent executes only user-context actions (userscripts/userfiles) upon daemon request.
func startAgentIPCServer(logger *utils.Logger, handler func(req ipc.RPCRequest) ipc.RPCResponse) (string, error) {
	if err := ipc.EnsureSocketDir(); err != nil {
		return "", err
	}

	// Determine UID to namespace the socket
	uid, err := getConsoleUserUID()
	if err != nil {
		return "", fmt.Errorf("failed to get console user uid: %w", err)
	}

	sockPath := ipc.GetAgentSocketPathForUID(uid)

	// Remove any stale socket
	_ = os.Remove(sockPath)

	l, err := net.Listen("unix", sockPath)
	if err != nil {
		return "", fmt.Errorf("failed to listen on %s: %w", sockPath, err)
	}

	// Set socket file permissions to allow the daemon (root) to connect
	// The socket file is owned by the agent user but readable/writable by root
	if err := os.Chmod(sockPath, 0666); err != nil {
		logger.Info("Failed to set socket permissions: %v", err)
	}

	logger.Info("Agent IPC listening at %s", sockPath)

	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				logger.Debug("IPC accept error: %v", err)
				time.Sleep(100 * time.Millisecond)
				continue
			}

			go func(c net.Conn) {
				defer c.Close()
				decoder := json.NewDecoder(bufio.NewReader(c))
				encoder := json.NewEncoder(c)

				var req ipc.RPCRequest
				if err := decoder.Decode(&req); err != nil {
					if err == io.EOF {
						// Likely a readiness probe (connect/close). Not an error.
						logger.Debug("IPC decode EOF (probe) - ignoring")
						return
					}
					logger.Error("IPC decode error: %v", err)
					return
				}

				logger.Debug("IPC request: id=%s cmd=%s path=%s donotwait=%t", req.ID, req.Command, req.Path, req.DoNotWait)
				resp := handler(req)
				if err := encoder.Encode(resp); err != nil {
					logger.Error("IPC encode error: %v", err)
				}
			}(conn)
		}
	}()

	return sockPath, nil
}

// getConsoleUserUID returns the current GUI console user UID for namespacing the socket.
func getConsoleUserUID() (string, error) {
	// Delegate to script executor method convention: stat -f %u /dev/console
	// Duplicate small helper to avoid import cycle
	out, err := utils.RunCommandCapture([]string{"stat", "-f", "%u", "/dev/console"})
	if err != nil {
		return "", err
	}
	return out, nil
}
