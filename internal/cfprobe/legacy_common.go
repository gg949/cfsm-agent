package cfprobe

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"
)

const (
	legacyShellScriptName   = "cf-probe.sh"
	legacyUnixScriptFile    = "/usr/local/bin/cf-probe.sh"
	legacyWindowsTaskName   = "CFProbe"
	legacyWindowsScriptName = "cf-server-monitor.ps1"
	legacyWindowsLogName    = "cf_probe.log"
)

func samePath(a, b string) bool {
	if a == "" || b == "" {
		return a == b
	}
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func removeFileIfExists(path string) bool {
	if path == "" {
		return false
	}
	if _, err := os.Lstat(path); err != nil {
		return false
	}
	_ = os.Remove(path)
	return true
}

func removeDirIfEmpty(path string) {
	if path == "" || path == "." || path == string(os.PathSeparator) {
		return
	}
	_ = os.Remove(path)
}

func splitCommandLineFields(raw string) []string {
	var fields []string
	var b strings.Builder
	inQuote := false
	quote := rune(0)
	escaped := false
	flush := func() {
		if b.Len() == 0 {
			return
		}
		fields = append(fields, b.String())
		b.Reset()
	}

	for _, r := range raw {
		if escaped {
			b.WriteRune(r)
			escaped = false
			continue
		}
		if r == '`' {
			escaped = true
			continue
		}
		if inQuote {
			if r == quote {
				inQuote = false
				continue
			}
			b.WriteRune(r)
			continue
		}
		if r == '"' || r == '\'' {
			inQuote = true
			quote = r
			continue
		}
		if unicode.IsSpace(r) {
			flush()
			continue
		}
		b.WriteRune(r)
	}
	flush()
	return fields
}

func extractLegacyPowerShellScriptPaths(raw string) []string {
	fields := splitCommandLineFields(raw)
	var paths []string
	for i := 0; i+1 < len(fields); i++ {
		if strings.EqualFold(fields[i], "-File") && strings.EqualFold(baseNameAnySep(fields[i+1]), legacyWindowsScriptName) {
			paths = append(paths, fields[i+1])
		}
	}
	return uniqueStrings(paths)
}

func baseNameAnySep(path string) string {
	path = strings.Trim(path, `"'`)
	idx := strings.LastIndexAny(path, `\/`)
	if idx >= 0 {
		return path[idx+1:]
	}
	return path
}
