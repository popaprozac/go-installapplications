package installer

import (
	"time"

	"github.com/go-installapplications/pkg/utils"
)

// Installer defines what an installer should be able to do
type Installer interface {
	InstallPackage(pkgPath, target string) error
	ExecuteScript(scriptPath, scriptType string, doNotWait bool, trackBackgroundProcesses bool) error
	ExecuteScriptForPreflight(scriptPath, scriptType string, doNotWait bool, trackBackgroundProcesses bool) error
	PlaceFile(filePath, fileType string) error
	WaitForBackgroundProcesses(timeout time.Duration) []error
	GetBackgroundProcessCount() int
}

// SystemInstaller combines package, script, and file installation
type SystemInstaller struct {
	packageInstaller *PackageInstaller
	scriptExecutor   *ScriptExecutor
	filePlacer       *FilePlacer
	logger           *utils.Logger
}

// NewSystemInstaller creates a new system installer
func NewSystemInstaller(dryRun bool, logger *utils.Logger, isAgentMode bool) *SystemInstaller {
	return &SystemInstaller{
		packageInstaller: NewPackageInstaller(dryRun, logger, isAgentMode),
		scriptExecutor:   NewScriptExecutor(dryRun, logger, isAgentMode),
		filePlacer:       NewFilePlacer(dryRun, logger, isAgentMode),
		logger:           logger,
	}
}

// InstallPackage installs a package
func (si *SystemInstaller) InstallPackage(pkgPath, target string) error {
	return si.packageInstaller.InstallPackage(pkgPath, target)
}

// ExecuteScript executes a script with donotwait and tracking support
func (si *SystemInstaller) ExecuteScript(scriptPath, scriptType string, doNotWait bool, trackBackgroundProcesses bool) error {
	return si.scriptExecutor.ExecuteScript(scriptPath, scriptType, doNotWait, trackBackgroundProcesses)
}

// ExecuteScriptForPreflight executes a script with special preflight exit code handling
func (si *SystemInstaller) ExecuteScriptForPreflight(scriptPath, scriptType string, doNotWait bool, trackBackgroundProcesses bool) error {
	return si.scriptExecutor.ExecuteScriptForPreflight(scriptPath, scriptType, doNotWait, trackBackgroundProcesses)
}

// PlaceFile places a file with appropriate permissions
func (si *SystemInstaller) PlaceFile(filePath, fileType string) error {
	return si.filePlacer.PlaceFile(filePath, fileType)
}

// WaitForBackgroundProcesses waits for all background processes to complete
func (si *SystemInstaller) WaitForBackgroundProcesses(timeout time.Duration) []error {
	return si.scriptExecutor.WaitForBackgroundProcesses(timeout)
}

// GetBackgroundProcessCount returns the number of active background processes
func (si *SystemInstaller) GetBackgroundProcessCount() int {
	return si.scriptExecutor.GetBackgroundProcessCount()
}
