package cfprobe

import (
	"strings"
	"testing"
)

func TestManagementCommandsSystemd(t *testing.T) {
	paths := Paths{ServiceName: "cf-probe"}
	cmds := managementCommands(paths, "systemd")
	want := []managementCommand{
		{labelRealtimeLog, "journalctl -u cf-probe -f"},
		{labelStatus, "systemctl status cf-probe"},
		{labelStop, "systemctl stop cf-probe"},
	}
	assertManagementCommands(t, cmds, want)
}

func TestManagementCommandsSystemdUser(t *testing.T) {
	paths := Paths{ServiceName: "cf-probe"}
	cmds := managementCommands(paths, "systemd-user")
	want := []managementCommand{
		{labelRealtimeLog, "journalctl --user -u cf-probe -f"},
		{labelStatus, "systemctl --user status cf-probe"},
		{labelStop, "systemctl --user stop cf-probe"},
	}
	assertManagementCommands(t, cmds, want)
}

func TestManagementCommandsOpenRC(t *testing.T) {
	paths := Paths{ServiceName: "cf-probe", LogFile: "/var/log/cf-probe.log"}
	cmds := managementCommands(paths, "openrc")
	want := []managementCommand{
		{labelRealtimeLog, "tail -f " + quoteShell("/var/log/cf-probe.log")},
		{labelStatus, "rc-service cf-probe status"},
		{labelStop, "rc-service cf-probe stop"},
	}
	assertManagementCommands(t, cmds, want)
}

func TestManagementCommandsProcd(t *testing.T) {
	paths := Paths{ServiceName: "cf-probe"}
	cmds := managementCommands(paths, "procd")
	want := []managementCommand{
		{labelRealtimeLog, "logread -f -e cf-probe"},
		{labelStatus, "/etc/init.d/cf-probe status"},
		{labelStop, "/etc/init.d/cf-probe stop"},
	}
	assertManagementCommands(t, cmds, want)
}

func TestManagementCommandsWindows(t *testing.T) {
	paths := Paths{
		ServiceName: "cf-probe",
		LogFile:     `C:\ProgramData\cf-probe\cf-probe.log`,
	}
	cmds := managementCommands(paths, "windows")
	want := []managementCommand{
		{labelRealtimeLog, `powershell -NoProfile -Command "Get-Content -Path 'C:\ProgramData\cf-probe\cf-probe.log' -Wait"`},
		{labelStatus, "schtasks /Query /TN cf-probe"},
		{labelStop, "schtasks /End /TN cf-probe"},
	}
	assertManagementCommands(t, cmds, want)
}

func TestFormatManagementCommandUsesCopyableCommandLine(t *testing.T) {
	cmd := managementCommand{
		label:   labelRealtimeLog,
		command: `powershell -NoProfile -Command "Get-Content -Path 'C:\ProgramData\cf-probe\cf-probe.log' -Wait"`,
	}
	got := formatManagementCommand(cmd)
	if strings.Contains(got, " : powershell") {
		t.Fatalf("command rendered on label line: %q", got)
	}
	if !strings.Contains(got, " :\n      powershell") {
		t.Fatalf("command not rendered on its own indented line: %q", got)
	}
}

func TestManagementCommandsLaunchdUser(t *testing.T) {
	paths := Paths{
		ServiceName:     "cf-probe",
		LogFile:         "/Users/alice/.cf-probe/cf-probe.log",
		LaunchdLabel:    "com.cfsm.cf-probe",
		LaunchdUserFile: "/Users/alice/Library/LaunchAgents/com.cfsm.cf-probe.plist",
		UserMode:        true,
		RunUID:          501,
	}
	cmds := managementCommands(paths, "launchd")
	want := []managementCommand{
		{labelRealtimeLog, "tail -f " + quoteShell("/Users/alice/.cf-probe/cf-probe.log")},
		{labelStatus, "launchctl print gui/501/com.cfsm.cf-probe"},
		{labelStop, "launchctl bootout gui/501 " + quoteShell("/Users/alice/Library/LaunchAgents/com.cfsm.cf-probe.plist")},
	}
	assertManagementCommands(t, cmds, want)
}

func assertManagementCommands(t *testing.T, got, want []managementCommand) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("cmd[%d] = %#v, want %#v", i, got[i], want[i])
		}
	}
}
