//go:build darwin && !windows

package cfprobe

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSudoUserHomeDirPrefersSudoUser(t *testing.T) {
	t.Setenv("SUDO_USER", "alice")
	t.Setenv("HOME", "/var/root")

	if got := sudoUserHomeDir(); got != "/Users/alice" {
		t.Fatalf("sudoUserHomeDir = %q, want /Users/alice", got)
	}
}

func TestSystemDefaultPathsDarwinLeavesLaunchdUserFileEmpty(t *testing.T) {
	paths := systemDefaultPaths()
	if paths.LaunchdUserFile != "" {
		t.Fatalf("LaunchdUserFile = %q, want empty for macOS system paths", paths.LaunchdUserFile)
	}
	if paths.LaunchdRootFile == "" {
		t.Fatal("LaunchdRootFile should be set for macOS system paths")
	}
}

func TestUserUninstallResidualsIncludesDarwinLegacyUserLog(t *testing.T) {
	home := t.TempDir()
	paths := userPaths("alice", 501, home)
	oldLog := filepath.Join(home, "Library", "Logs", "cf-probe.log")
	if err := os.MkdirAll(filepath.Dir(oldLog), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(oldLog, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	residuals := userUninstallResiduals(paths)
	if !containsString(residuals, oldLog) {
		t.Fatalf("residuals = %#v, want old log %s", residuals, oldLog)
	}
}

func TestRemoveInstalledFilesRemovesDarwinLegacyUserLog(t *testing.T) {
	home := t.TempDir()
	paths := userPaths("alice", 501, home)
	oldLog := filepath.Join(home, "Library", "Logs", "cf-probe.log")
	if err := os.MkdirAll(filepath.Dir(oldLog), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(oldLog, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	removeInstalledFiles(paths)

	if _, err := os.Stat(oldLog); !os.IsNotExist(err) {
		t.Fatalf("legacy log should be removed, stat err = %v", err)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
