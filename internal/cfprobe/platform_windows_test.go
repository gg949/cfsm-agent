//go:build windows

package cfprobe

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteWindowsTaskWrapperRedirectsOutputWithoutOpeningLog(t *testing.T) {
	tmp := t.TempDir()
	paths := Paths{
		ServiceName: "cf-probe",
		BinaryFile:  filepath.Join(tmp, "Program Files", "cf-probe", "cf-probe.exe"),
		ConfigDir:   filepath.Join(tmp, "ProgramData", "cf-probe"),
		LogFile:     filepath.Join(tmp, "ProgramData", "cf-probe", "cf-probe.log"),
	}

	if err := writeWindowsTaskWrapper(paths, true); err != nil {
		t.Fatalf("writeWindowsTaskWrapper returned error: %v", err)
	}
	if _, err := os.Stat(paths.LogFile); !os.IsNotExist(err) {
		t.Fatalf("log file should not be opened or created while writing wrapper: %v", err)
	}
	data, err := os.ReadFile(windowsTaskWrapperFile(paths))
	if err != nil {
		t.Fatalf("read wrapper: %v", err)
	}
	content := string(data)
	wantRun := quoteWindowsCmdArg(paths.BinaryFile) + " run -debug=1 >> " + quoteWindowsCmdArg(paths.LogFile) + " 2>&1"
	if !strings.Contains(content, wantRun) {
		t.Fatalf("wrapper missing redirected run command %q:\n%s", wantRun, content)
	}
}

func TestWindowsServiceArgsRunsTaskWrapper(t *testing.T) {
	paths := Paths{
		ServiceName: "cf-probe",
		ConfigDir:   `C:\ProgramData\cf-probe`,
	}

	got := strings.Join(windowsServiceArgs(paths, false), " ")
	want := `cmd.exe /D /C "C:\ProgramData\cf-probe\cf-probe.cmd"`
	if got != want {
		t.Fatalf("windowsServiceArgs = %q, want %q", got, want)
	}
}

func TestWindowsScheduledTaskXMLStartsOnBootLogonAndResume(t *testing.T) {
	paths := Paths{
		ServiceName: "cf-probe",
		ConfigDir:   `C:\ProgramData\cf-probe`,
	}

	content, err := windowsScheduledTaskXML(paths, false)
	if err != nil {
		t.Fatalf("windowsScheduledTaskXML returned error: %v", err)
	}
	for _, want := range []string{
		`<BootTrigger>`,
		`<LogonTrigger>`,
		`<EventTrigger>`,
		`Microsoft-Windows-Power-Troubleshooter`,
		`EventID=1`,
		`Microsoft-Windows-Kernel-Power`,
		`EventID=107`,
		`<Delay>PT10S</Delay>`,
		`<UserId>S-1-5-18</UserId>`,
		`<RunLevel>HighestAvailable</RunLevel>`,
		`<MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>`,
		`<StartWhenAvailable>true</StartWhenAvailable>`,
		`<DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>`,
		`<StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>`,
		`<ExecutionTimeLimit>PT0S</ExecutionTimeLimit>`,
		`<Command>cmd.exe</Command>`,
		`<Arguments>/D /C &#34;C:\ProgramData\cf-probe\cf-probe.cmd&#34;</Arguments>`,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("task XML missing %q:\n%s", want, content)
		}
	}
	if strings.Contains(content, "<LogonType>") {
		t.Fatalf("task XML should rely on /RU SYSTEM and omit LogonType for schtasks compatibility:\n%s", content)
	}
}

func TestUTF16LEWithBOM(t *testing.T) {
	got := utf16LEWithBOM("AB")
	want := []byte{0xff, 0xfe, 'A', 0x00, 'B', 0x00}
	if !bytes.Equal(got, want) {
		t.Fatalf("utf16LEWithBOM = %v, want %v", got, want)
	}
}
