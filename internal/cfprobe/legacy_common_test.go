package cfprobe

import "testing"

func TestExtractLegacyPowerShellScriptPaths(t *testing.T) {
	raw := `powershell.exe
-NoProfile -ExecutionPolicy Bypass -STA -WindowStyle Hidden -File "C:\Probe Dir\cf-server-monitor.ps1" run -STA`

	got := extractLegacyPowerShellScriptPaths(raw)
	if len(got) != 1 || got[0] != `C:\Probe Dir\cf-server-monitor.ps1` {
		t.Fatalf("paths = %#v", got)
	}
}

func TestExtractLegacyPowerShellScriptPathsIgnoresOtherScripts(t *testing.T) {
	raw := `powershell.exe -File "C:\Probe Dir\other.ps1" run -STA`
	if got := extractLegacyPowerShellScriptPaths(raw); len(got) != 0 {
		t.Fatalf("paths = %#v, want none", got)
	}
}
