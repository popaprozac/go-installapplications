package mode

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/go-installapplications/pkg/ipc"
	"github.com/go-installapplications/pkg/utils"
)

func generateRequestID() string {
	// 8 random bytes + timestamp suffix
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("req-%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("req-%s-%d", hex.EncodeToString(b), time.Now().UnixNano())
}

// waitForAgentSocket waits until the agent socket is available or times out.
// This replaces the older file-based "userland ready" signal and is more reliable.
func waitForAgentSocket(logger *utils.Logger, timeout time.Duration) (string, error) {
	uid, err := getConsoleUserUID()
	if err != nil {
		return "", err
	}
	sockPath := ipc.GetAgentSocketPathForUID(uid)

	logger.Debug("Waiting for agent socket: %s", sockPath)
	start := time.Now()
	for {
		conn, err := net.DialTimeout("unix", sockPath, 2*time.Second)
		if err == nil {
			_ = conn.Close()
			logger.Info("Agent socket is ready")
			return sockPath, nil
		}
		if time.Since(start) > timeout {
			return "", fmt.Errorf("timeout waiting for agent socket: %s", sockPath)
		}
		time.Sleep(1 * time.Second)
	}
}

// callAgent sends an RPC to the agent and waits for a response.
// Requests are synchronous to preserve strict item ordering across userland.
func callAgent(logger *utils.Logger, sockPath string, req ipc.RPCRequest, callTimeout time.Duration) (ipc.RPCResponse, error) {
	// ensure request id
	if req.ID == "" {
		req.ID = generateRequestID()
	}

	conn, err := net.DialTimeout("unix", sockPath, 2*time.Second)
	if err != nil {
		return ipc.RPCResponse{}, fmt.Errorf("failed to connect agent: %w", err)
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(callTimeout))

	enc := json.NewEncoder(conn)
	dec := json.NewDecoder(bufio.NewReader(conn))

	logger.Debug("Sending IPC request id=%s cmd=%s", req.ID, req.Command)
	if err := enc.Encode(req); err != nil {
		return ipc.RPCResponse{}, fmt.Errorf("encode error: %w", err)
	}
	var resp ipc.RPCResponse
	if err := dec.Decode(&resp); err != nil {
		return ipc.RPCResponse{}, fmt.Errorf("decode error: %w", err)
	}
	if resp.ID != req.ID {
		return ipc.RPCResponse{}, fmt.Errorf("mismatched response id")
	}
	return resp, nil
}
