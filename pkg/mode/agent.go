package mode

import (
	"github.com/go-installapplications/pkg/config"
	"github.com/go-installapplications/pkg/installer"
	"github.com/go-installapplications/pkg/ipc"
	"github.com/go-installapplications/pkg/utils"
)

// RunAgent executes the agent mode workflow
func RunAgent(cfg *config.Config, logger *utils.Logger) {
	logger.Info("Starting agent mode")

	// Start IPC server to receive requests from daemon for user-context actions
	done := make(chan struct{})
	_, err := startAgentIPCServer(logger, func(req ipc.RPCRequest) ipc.RPCResponse {
		switch req.Command {
		case "Ping":
			return ipc.RPCResponse{ID: req.ID, OK: true}
		case "Shutdown":
			// Graceful shutdown
			go func() { close(done) }()
			return ipc.RPCResponse{ID: req.ID, OK: true}
		case "RunUserScript":
			installer := installer.NewSystemInstaller(cfg.DryRun, logger, true)
			if req.DoNotWait {
				// For now, we treat donotwait as immediate start; background tracking remains local
				if err := installer.ExecuteScript(req.Path, "userscript", true, cfg.TrackBackgroundProcesses); err != nil {
					return ipc.RPCResponse{ID: req.ID, OK: false, Error: err.Error()}
				}
				return ipc.RPCResponse{ID: req.ID, OK: true, Started: true}
			}
			if err := installer.ExecuteScript(req.Path, "userscript", false, cfg.TrackBackgroundProcesses); err != nil {
				return ipc.RPCResponse{ID: req.ID, OK: false, Error: err.Error()}
			}
			return ipc.RPCResponse{ID: req.ID, OK: true}
		case "PlaceUserFile":
			installer := installer.NewSystemInstaller(cfg.DryRun, logger, true)
			if err := installer.PlaceFile(req.Path, "userfile"); err != nil {
				return ipc.RPCResponse{ID: req.ID, OK: false, Error: err.Error()}
			}
			return ipc.RPCResponse{ID: req.ID, OK: true}
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
