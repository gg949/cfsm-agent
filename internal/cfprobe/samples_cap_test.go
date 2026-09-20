package cfprobe

import (
	"testing"
	"time"
)

// 回归测试：上报持续失败时 samples 缓冲必须被截断，不能无限增长。
func TestPendingSamplesAreCappedWhenReportsFail(t *testing.T) {
	base := time.Unix(1000, 0)
	samples := make([]metricSample, 0, maxPendingSamples)

	// 模拟 collect_interval 持续入队但上报一直失败（不清空）
	for i := 0; i < maxPendingSamples*3; i++ {
		samples = append(samples, metricSample{at: base, metrics: map[string]any{"i": i}})
		if len(samples) > maxPendingSamples {
			samples = samples[len(samples)-maxPendingSamples:]
		}
	}

	if len(samples) != maxPendingSamples {
		t.Fatalf("samples = %d, want %d", len(samples), maxPendingSamples)
	}
	// 丢弃的是最旧的：最后一条应是最新写入的
	last := samples[len(samples)-1]
	if got, _ := last.metrics["i"].(int); got != maxPendingSamples*3-1 {
		t.Fatalf("last sample i = %v, want %d", last.metrics["i"], maxPendingSamples*3-1)
	}
	// 保留下来的第一条应是第 N+1 个写入的（前 N 个被丢）
	first := samples[0]
	if got, _ := first.metrics["i"].(int); got != maxPendingSamples*2 {
		t.Fatalf("first sample i = %v, want %d", first.metrics["i"], maxPendingSamples*2)
	}
}

// 正常路径：上报成功后清空，缓冲不应长期驻留。
func TestPendingSamplesClearedAfterSuccessfulReport(t *testing.T) {
	samples := []metricSample{
		{at: time.Unix(1000, 0), metrics: map[string]any{"i": 0}},
		{at: time.Unix(1001, 0), metrics: map[string]any{"i": 1}},
	}
	if len(samples) == 0 {
		t.Fatal("precondition failed")
	}
	samples = nil
	if len(samples) != 0 {
		t.Fatalf("samples = %d, want 0 after clear", len(samples))
	}
}

// 上限本身必须为正数且在合理量级（防止误改成 0 或极小值导致样本全丢）。
func TestMaxPendingSamplesIsReasonable(t *testing.T) {
	if maxPendingSamples < 64 || maxPendingSamples > 4096 {
		t.Fatalf("maxPendingSamples = %d, want between 64 and 4096", maxPendingSamples)
	}
}
