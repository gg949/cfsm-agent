//go:build darwin

package cfprobe

import "testing"

func TestParseDarwinTopCPUPercent(t *testing.T) {
	value, ok := parsePercentBeforeMark("  87.50% idle")
	if !ok {
		t.Fatal("expected percentage to parse")
	}
	if value != 87.5 {
		t.Fatalf("percentage = %v, want 87.5", value)
	}
}

func TestParseDarwinIORegGPUStats(t *testing.T) {
	raw := `+-o AGXAccelerator  <class AGXAccelerator, id 0x1>
    {
      "PerformanceStatistics" = {"Device Utilization %"=52,"Renderer Utilization %"=48}
      "model" = "Apple M5"
    }`

	if got := parseIORegQuotedValue(raw, "model"); got != "Apple M5" {
		t.Fatalf("model = %q, want Apple M5", got)
	}

	values := parseIORegNumberValues(raw, "Device Utilization %")
	if len(values) != 1 || values[0] != 52 {
		t.Fatalf("values = %#v, want [52]", values)
	}
}
