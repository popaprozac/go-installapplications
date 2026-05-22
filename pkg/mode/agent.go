package mode

import (
	"sync"
	"time"

	"github.com/go-installapplications/pkg/config"
	"github.com/go-installapplications/pkg/installer"
	"github.com/go-installapplications/pkg/ipc"
	"github.com/go-installapplications/pkg/utils"
)

// RunAgent executes the agent mode workflow
func RunAgent(cfg *config.Config, logger *utils.Logger) {
	logger.Info("Starting agent mode")

	// A single long-lived SystemInstaller so its ProcessTracker survives across
	// requests. Without this, donotwait userscripts would be tracked in a
	// per-request tracker that gets GC'd, defeating TrackBackgroundProcesses.
	systemInstaller := installer.NewSystemInstaller(cfg.DryRun, logger, true)

	// Start IPC server to receive requests from daemon for user-context actions.
	// shutdownOnce guards close(done) so repeated Shutdown commands cannot panic.
	done := make(chan struct{})
	var shutdownOnce sync.Once
	_, err := startAgentIPCServer(logger, func(req ipc.RPCRequest) ipc.RPCResponse {
		switch req.Command {
		case "Ping":
			return ipc.RPCResponse{ID: req.ID, OK: true}
		case "Shutdown":
			// Graceful shutdown — idempotent across repeated Shutdown calls
			shutdownOnce.Do(func() { close(done) })
			return ipc.RPCResponse{ID: req.ID, OK: true}
		case "RunUserScript":
			if err := systemInstaller.ExecuteScript(req.Path, "userscript", req.DoNotWait, cfg.TrackBackgroundProcesses); err != nil {
				return ipc.RPCResponse{ID: req.ID, OK: false, Error: err.Error()}
			}
			return ipc.RPCResponse{ID: req.ID, OK: true, Started: req.DoNotWait}
		case "PlaceUserFile":
			if err := systemInstaller.PlaceFile(req.Path, "userfile"); err != nil {
				return ipc.RPCResponse{ID: req.ID, OK: false, Error: err.Error()}
			}
			return ipc.RPCResponse{ID: req.ID, OK: true}
		case "GetBackgroundProcessCount":
			return ipc.RPCResponse{ID: req.ID, OK: true, Count: systemInstaller.GetBackgroundProcessCount()}
		case "WaitForBackgroundProcesses":
			timeout := time.Duration(req.TimeoutSeconds) * time.Second
			if timeout <= 0 {
				timeout = cfg.BackgroundTimeout
			}
			errs := systemInstaller.WaitForBackgroundProcesses(timeout)
			if len(errs) == 0 {
				return ipc.RPCResponse{ID: req.ID, OK: true}
			}
			strs := make([]string, 0, len(errs))
			for _, e := range errs {
				strs = append(strs, e.Error())
			}
			return ipc.RPCResponse{ID: req.ID, OK: false, Errors: strs, Error: strs[0]}
		default:
			return ipc.RPCResponse{ID: req.ID, OK: false, Error: "unknown command"}
		}
	})
	if err != nil {
		logger.Error("Failed to start agent IPC: %v", err)
		utils.Exit(cfg, logger, 1, "failed to start agent IPC")
	}

	// Keep the agent process alive until a shutdown request is received
	<-done
}
