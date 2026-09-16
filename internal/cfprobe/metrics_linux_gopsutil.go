//go:build linux && (amd64 || arm64 || 386 || arm || loong64)

package cfprobe

import (
	gopsutilCPU "github.com/shirou/gopsutil/v4/cpu"
)

func readGopsutilCPUPercent() (float64, bool) {
	percentages, err := gopsutilCPU.Percent(0, false)
	if err != nil || len(percentages) == 0 {
		return 0, false
	}
	return percentages[0], true
}
