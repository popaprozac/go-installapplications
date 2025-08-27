package manager

import (
	"errors"
	"testing"
	"time"

	"github.com/go-installapplications/pkg/config"
	"github.com/go-installapplications/pkg/download"
	"github.com/go-installapplications/pkg/installer"
	"github.com/go-installapplications/pkg/utils"
)

// fake downloader installs nothing; just returns success
type fakeDownloader struct{}

func (f *fakeDownloader) DownloadFile(u, p, h string) error                      { return nil }
func (f *fakeDownloader) DownloadFileWithRetries(u, p, h string, r, w int) error { return nil }
func (f *fakeDownloader) VerifyFileHash(p, h string) error                       { return nil }

func (f *fakeDownloader) DownloadMultipleWithCleanup(items []config.Item, max int, cleanup bool) []download.DownloadResult {
	out := make([]download.DownloadResult, len(items))
	for i, it := range items {
		out[i] = download.DownloadResult{Item: it, Error: nil}
	}
	return out
}

// fake installer tracks calls
type fakeInstaller struct{ scripts int }

func (f *fakeInstaller) InstallPackage(pkgPath, target string) error { return nil }
func (f *fakeInstaller) ExecuteScript(scriptPath, scriptType string, doNotWait bool, track bool) error {
	f.scripts++
	if scriptPath == "fail.sh" {
		return errors.New("boom")
	}
	return nil
}
func (f *fakeInstaller) ExecuteScriptForPreflight(scriptPath, scriptType string, doNotWait bool, track bool) error {
	f.scripts++
	if scriptPath == "fail.sh" {
		return errors.New("boom")
	}
	return nil
}
func (f *fakeInstaller) PlaceFile(filePath, fileType string) error                { return nil }
func (f *fakeInstaller) WaitForBackgroundProcesses(timeout time.Duration) []error { return nil }
func (f *fakeInstaller) GetBackgroundProcessCount() int                           { return 0 }

var _ installer.Installer = (*fakeInstaller)(nil)

func TestManagerProcessItems_FailPolicy(t *testing.T) {
	dl := &fakeDownloader{}
	inst := &fakeInstaller{}
	cfg := config.NewConfig()
	logger := utils.NewLogger(false, false)

	m := NewManager(dl, inst, cfg, logger)

	items := []config.Item{
		{Name: "good", File: "ok.sh", Type: "rootscript"},
		{Name: "bad", File: "fail.sh", Type: "rootscript", FailPolicy: "failable_execution"},
		{Name: "stop", File: "fail.sh", Type: "rootscript", FailPolicy: "failure_is_not_an_option"},
	}
	if err := m.ProcessItems(items, "userland"); err == nil {
		t.Fatalf("expected error due to last item policy")
	}
	if inst.scripts < 2 {
		t.Fatalf("expected at least two script executions")
	}
}
