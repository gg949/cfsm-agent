//go:build windows

package cfprobe

import (
	"os"
	"path/filepath"
	"strings"
)

func cleanupLegacyInstall(paths Paths) ([]string, []string) {
	actions := strings.TrimSpace(legacyWindowsTaskActions() + "\n" + legacyWindowsProcessCommandLines())
	scripts := extractLegacyPowerShellScriptPaths(actions)
	if !legacyWindowsInstallDetected(actions) {
		return nil, nil
	}

	var cleaned []string
	if runCommandQuiet("schtasks", "/Query", "/TN", legacyWindowsTaskName) == nil {
		_ = runCommandQuiet("schtasks", "/End", "/TN", legacyWindowsTaskName)
		_ = runCommandQuiet("schtasks", "/Delete", "/TN", legacyWindowsTaskName, "/F")
		cleaned = append(cleaned, "scheduled-task:"+legacyWindowsTaskName)
	}
	if legacyWindowsProcessRunning() {
		stopLegacyWindowsProcesses()
		cleaned = append(cleaned, "process:"+legacyWindowsScriptName)
	}
	for _, script := range scripts {
		if removeFileIfExists(script) {
			cleaned = append(cleaned, script)
		}
		logFile := filepath.Join(filepath.Dir(script), legacyWindowsLogName)
		if removeFileIfExists(logFile) {
			cleaned = append(cleaned, logFile)
		}
	}
	tempPing := filepath.Join(os.TempDir(), "cf_probe_ping_results.json")
	if removeFileIfExists(tempPing) {
		cleaned = append(cleaned, tempPing)
	}

	return uniqueStrings(cleaned), legacyWindowsResiduals(scripts)
}

func legacyConfigFiles(_ Paths) []string {
	return nil
}

func legacyTrafficFiles(paths Paths) []string {
	return []string{paths.OldTrafficFile}
}

func legacyWindowsInstallDetected(taskActions string) bool {
	return taskActions != "" || legacyWindowsProcessRunning()
}

func legacyWindowsTaskActions() string {
	return commandOutput("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command",
		"$t=Get-ScheduledTask -TaskName '"+legacyWindowsTaskName+"' -ErrorAction SilentlyContinue; if($t){$t.Actions | ForEach-Object { $_.Execute; $_.Arguments }}")
}

func legacyWindowsProcessRunning() bool {
	return commandOutput("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command",
		"$me=$PID; Get-CimInstance Win32_Process | Where-Object { $_.ProcessId -ne $me -and $_.CommandLine -like '*cf-server-monitor*run*' } | Select-Object -First 1 -ExpandProperty ProcessId") != ""
}

func legacyWindowsProcessCommandLines() string {
	return commandOutput("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command",
		"$me=$PID; Get-CimInstance Win32_Process | Where-Object { $_.ProcessId -ne $me -and $_.CommandLine -like '*cf-server-monitor*run*' } | Select-Object -ExpandProperty CommandLine")
}

func stopLegacyWindowsProcesses() {
	_ = runCommandQuiet("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command",
		"$ErrorActionPreference='SilentlyContinue'; $me=$PID; Get-CimInstance Win32_Process | Where-Object { $_.ProcessId -ne $me -and $_.CommandLine -like '*cf-server-monitor*run*' } | ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }")
}

func legacyWindowsResiduals(scripts []string) []string {
	var residuals []string
	if runCommandQuiet("schtasks", "/Query", "/TN", legacyWindowsTaskName) == nil {
		residuals = append(residuals, "scheduled-task:"+legacyWindowsTaskName)
	}
	if legacyWindowsProcessRunning() {
		residuals = append(residuals, "process:"+legacyWindowsScriptName)
	}
	for _, script := range scripts {
		if fileExists(script) {
			residuals = append(residuals, script)
		}
	}
	return uniqueStrings(residuals)
}
