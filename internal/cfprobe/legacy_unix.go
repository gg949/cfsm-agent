//go:build !windows

package cfprobe

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const legacyDarwinLaunchdFile = "/Library/LaunchDaemons/com.cf.probe.plist"

func cleanupLegacyInstall(paths Paths) ([]string, []string) {
	if !legacyUnixInstallDetected(paths) {
		return nil, nil
	}

	var cleaned []string
	stopLegacyUnixAutostart(paths)
	if legacyShellProcessRunning() {
		stopLegacyShellProcesses()
		cleaned = append(cleaned, "process:"+legacyShellScriptName)
	}
	for _, pidFile := range legacyUnixPIDFiles(paths) {
		if removeFileIfExists(pidFile) {
			cleaned = append(cleaned, pidFile)
		}
	}
	if removeFileIfExists(legacyUnixScriptFile) {
		cleaned = append(cleaned, legacyUnixScriptFile)
	}
	if removeFileIfExists(legacyUnixScriptFile + ".ctl") {
		cleaned = append(cleaned, legacyUnixScriptFile+".ctl")
	}
	for _, serviceFile := range legacyUnixServiceFiles(paths) {
		if legacyFileContains(serviceFile, legacyShellScriptName) && removeFileIfExists(serviceFile) {
			cleaned = append(cleaned, serviceFile)
		}
	}
	if runtime.GOOS == "darwin" && removeFileIfExists(legacyDarwinLaunchdFile) {
		cleaned = append(cleaned, legacyDarwinLaunchdFile)
	}
	for _, path := range legacyRuntimeFiles() {
		if removeFileIfExists(path) {
			cleaned = append(cleaned, path)
		}
	}
	if removeLegacyVarLibExceptTraffic(paths) {
		cleaned = append(cleaned, filepath.Dir(paths.OldTrafficFile))
	}
	for _, logFile := range legacyUnixLogFiles(paths) {
		if removeFileIfExists(logFile) {
			cleaned = append(cleaned, logFile)
		}
	}
	if removeFileIfExists(paths.DebugEnvFile) {
		cleaned = append(cleaned, paths.DebugEnvFile)
	}

	if commandExists("systemctl") {
		_ = runCommandQuiet("systemctl", "daemon-reload")
		_ = runCommandQuiet("systemctl", "reset-failed", paths.ServiceName)
		_ = runCommandQuiet("systemctl", "reset-failed", paths.ServiceName+".service")
	}

	return uniqueStrings(cleaned), legacyUnixResiduals(paths)
}

func legacyConfigFiles(paths Paths) []string {
	candidates := []string{paths.ConfigFile}
	if runtime.GOOS == "darwin" {
		candidates = append(candidates, "/Library/Application Support/cf-probe/config.conf")
	}
	if isSynology() {
		candidates = append(candidates, "/usr/local/etc/cf-probe/config.conf")
	}
	return uniqueStrings(candidates)
}

func legacyTrafficFiles(paths Paths) []string {
	candidates := []string{paths.OldTrafficFile}
	if runtime.GOOS == "darwin" {
		candidates = append(candidates, "/Library/Application Support/cf-probe/traffic.dat")
	}
	if isSynology() {
		candidates = append(candidates, "/usr/local/etc/cf-probe/traffic.dat")
	}
	return uniqueStrings(candidates)
}

func legacyUnixInstallDetected(paths Paths) bool {
	if fileExists(legacyUnixScriptFile) || fileExists(legacyUnixScriptFile+".ctl") {
		return true
	}
	for _, serviceFile := range legacyUnixServiceFiles(paths) {
		if legacyFileContains(serviceFile, legacyShellScriptName) {
			return true
		}
	}
	if runtime.GOOS == "darwin" && fileExists(legacyDarwinLaunchdFile) {
		return true
	}
	if legacyShellProcessRunning() {
		return true
	}
	return len(legacyRuntimeFiles()) > 0 || legacyVarLibHasRemovableFiles(paths)
}

func stopLegacyUnixAutostart(paths Paths) {
	if runtime.GOOS == "darwin" {
		bootoutLaunchd("system", legacyDarwinLaunchdFile)
		bootoutLaunchdLabel("system", "com.cf.probe")
		return
	}
	if legacyFileContains(paths.ServiceFile, legacyShellScriptName) && commandExists("systemctl") {
		_ = runCommandQuiet("systemctl", "stop", paths.ServiceName+".service")
		_ = runCommandQuiet("systemctl", "disable", paths.ServiceName+".service")
	}
	initScript := filepath.Join("/etc/init.d", paths.ServiceName)
	if legacyFileContains(initScript, legacyShellScriptName) {
		_ = runCommandQuiet(initScript, "stop")
		_ = runCommandQuiet(initScript, "disable")
		if commandExists("rc-service") {
			_ = runCommandQuiet("rc-service", paths.ServiceName, "stop")
		}
		if commandExists("rc-update") {
			_ = runCommandQuiet("rc-update", "del", paths.ServiceName, "default")
		}
	}
	upstartFile := filepath.Join("/etc/init", paths.ServiceName+".conf")
	if legacyFileContains(upstartFile, legacyShellScriptName) && commandExists("initctl") {
		_ = runCommandQuiet("initctl", "stop", paths.ServiceName)
	}
	synologyFile := synologyServiceFile(paths)
	if legacyFileContains(synologyFile, legacyShellScriptName) {
		_ = runCommandQuiet(synologyFile, "stop")
	}
}

func legacyUnixServiceFiles(paths Paths) []string {
	return uniqueStrings([]string{
		paths.ServiceFile,
		filepath.Join("/etc/init.d", paths.ServiceName),
		filepath.Join("/etc/init", paths.ServiceName+".conf"),
		synologyServiceFile(paths),
	})
}

func legacyUnixPIDFiles(paths Paths) []string {
	return uniqueStrings([]string{
		paths.PIDFile,
		filepath.Join("/run", paths.ServiceName+".pid"),
		filepath.Join("/var/run", paths.ServiceName+".pid"),
	})
}

func legacyUnixLogFiles(paths Paths) []string {
	return uniqueStrings([]string{
		paths.LogFile,
		filepath.Join("/var/log", paths.ServiceName+".log"),
	})
}

func legacyFileContains(path, needle string) bool {
	data, err := os.ReadFile(path)
	return err == nil && strings.Contains(string(data), needle)
}

func legacyShellProcessRunning() bool {
	return len(legacyShellProcessIDs()) > 0
}

func stopLegacyShellProcesses() {
	for _, pid := range legacyShellProcessIDs() {
		if proc, findErr := os.FindProcess(pid); findErr == nil {
			_ = proc.Kill()
		}
	}
	for i := 0; i < 20; i++ {
		if !legacyShellProcessRunning() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func legacyShellProcessIDs() []int {
	if !commandExists("pgrep") {
		return nil
	}
	return parseLegacyShellProcessIDs(commandOutput("pgrep", "-f", legacyUnixScriptFile), legacyProcessInCurrentNamespaces)
}

func parseLegacyShellProcessIDs(raw string, include func(int) bool) []int {
	var pids []int
	seen := map[int]bool{}
	for _, raw := range strings.Fields(raw) {
		pid, err := strconv.Atoi(raw)
		if err != nil || pid <= 0 || pid == os.Getpid() || seen[pid] || !include(pid) {
			continue
		}
		seen[pid] = true
		pids = append(pids, pid)
	}
	return pids
}

func legacyProcessInCurrentNamespaces(pid int) bool {
	if runtime.GOOS != "linux" {
		return true
	}
	for _, ns := range []string{"pid", "mnt"} {
		selfNS, err := os.Readlink(filepath.Join("/proc/self/ns", ns))
		if err != nil {
			return true
		}
		procNS, err := os.Readlink(filepath.Join("/proc", strconv.Itoa(pid), "ns", ns))
		if err != nil || procNS != selfNS {
			return false
		}
	}
	return true
}

func legacyRuntimeFiles() []string {
	var files []string
	for _, pattern := range []string{
		"/dev/shm/.cf_ipv4*",
		"/dev/shm/.cf_ipv6*",
		"/dev/shm/.cf_probe_*",
		"/tmp/.cf_ipv4*",
		"/tmp/.cf_ipv6*",
		"/tmp/.cf_probe_*",
		"/tmp/cf-probe/.cf_probe_*",
	} {
		matches, _ := filepath.Glob(pattern)
		files = append(files, matches...)
	}
	return uniqueStrings(files)
}

func legacyVarLibHasRemovableFiles(paths Paths) bool {
	dir := filepath.Dir(paths.OldTrafficFile)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if !samePath(path, paths.OldTrafficFile) {
			return true
		}
	}
	return false
}

func removeLegacyVarLibExceptTraffic(paths Paths) bool {
	dir := filepath.Dir(paths.OldTrafficFile)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	changed := false
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if samePath(path, paths.OldTrafficFile) {
			continue
		}
		_ = os.RemoveAll(path)
		changed = true
	}
	if !fileExists(paths.OldTrafficFile) {
		removeDirIfEmpty(dir)
	}
	return changed
}

func legacyUnixResiduals(paths Paths) []string {
	var residuals []string
	if fileExists(legacyUnixScriptFile) {
		residuals = append(residuals, legacyUnixScriptFile)
	}
	if fileExists(legacyUnixScriptFile + ".ctl") {
		residuals = append(residuals, legacyUnixScriptFile+".ctl")
	}
	for _, serviceFile := range legacyUnixServiceFiles(paths) {
		if legacyFileContains(serviceFile, legacyShellScriptName) {
			residuals = append(residuals, serviceFile)
		}
	}
	if runtime.GOOS == "darwin" && fileExists(legacyDarwinLaunchdFile) {
		residuals = append(residuals, legacyDarwinLaunchdFile)
	}
	if legacyShellProcessRunning() {
		residuals = append(residuals, "process:"+legacyShellScriptName)
	}
	residuals = append(residuals, legacyRuntimeFiles()...)
	if legacyVarLibHasRemovableFiles(paths) {
		residuals = append(residuals, filepath.Dir(paths.OldTrafficFile))
	}
	return uniqueStrings(residuals)
}
