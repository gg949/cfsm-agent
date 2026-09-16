package cfprobe

import "testing"

func TestCPUUsagePercentFromZeroPrevious(t *testing.T) {
	got, ok := cpuUsagePercent(cpuTimes{}, cpuTimes{Total: 100, Idle: 75})
	if !ok {
		t.Fatal("expected cpuUsagePercent to calculate from zero previous sample")
	}
	if got != 25 {
		t.Fatalf("got %.2f, want 25", got)
	}
}

func TestCPUPercentStringReportsZeroAsZero(t *testing.T) {
	if got := cpuPercentString(0); got != "0.00" {
		t.Fatalf("got %q, want 0.00", got)
	}
}

func TestCPUPercentStringReportsPercentUnits(t *testing.T) {
	if got := cpuPercentString(5); got != "5.00" {
		t.Fatalf("got %q, want 5.00", got)
	}
}

func TestDiskUsageMBFromBlocksUsesFreeBlocksForUsedValue(t *testing.T) {
	total, used, ok := diskUsageMBFromBlocks(100, 65, int64(bytesPerMiB))
	if !ok {
		t.Fatal("expected disk usage calculation to succeed")
	}
	if total != 100 {
		t.Fatalf("total = %d, want 100", total)
	}
	if used != 35 {
		t.Fatalf("used = %d, want 35", used)
	}
}

func TestDiskIOStatsFromCounters(t *testing.T) {
	prev := DiskIOCounters{
		ReadBytes:   1024,
		WriteBytes:  2048,
		ReadOps:     10,
		WriteOps:    20,
		ReadTimeMS:  100,
		WriteTimeMS: 300,
		IOTicksMS:   1000,
		DeviceCount: 2,
		Fingerprint: "8:1,8:2",
	}
	current := DiskIOCounters{
		ReadBytes:   3072,
		WriteBytes:  6144,
		ReadOps:     14,
		WriteOps:    28,
		ReadTimeMS:  180,
		WriteTimeMS: 480,
		IOTicksMS:   1600,
		DeviceCount: 2,
		Fingerprint: "8:1,8:2",
	}

	got := diskIOStatsFromCounters(prev, current, 2)
	if got.ReadBps != 1024 {
		t.Fatalf("read_bps = %d, want 1024", got.ReadBps)
	}
	if got.WriteBps != 2048 {
		t.Fatalf("write_bps = %d, want 2048", got.WriteBps)
	}
	if got.ReadIOPS != 2 {
		t.Fatalf("read_iops = %.2f, want 2", got.ReadIOPS)
	}
	if got.WriteIOPS != 4 {
		t.Fatalf("write_iops = %.2f, want 4", got.WriteIOPS)
	}
	if got.AwaitMS != 21.67 {
		t.Fatalf("await_ms = %.2f, want 21.67", got.AwaitMS)
	}
	if got.Util != 15 {
		t.Fatalf("util = %.2f, want 15", got.Util)
	}
}

func TestDiskIOStatsFromCountersRejectsChangedDeviceSet(t *testing.T) {
	prev := DiskIOCounters{ReadBytes: 1024, DeviceCount: 1, Fingerprint: "8:1"}
	current := DiskIOCounters{ReadBytes: 4096, DeviceCount: 1, Fingerprint: "8:2"}

	got := diskIOStatsFromCounters(prev, current, 30)
	if got != (DiskIOStats{}) {
		t.Fatalf("got %+v, want zero stats", got)
	}
}

func TestShouldIncludeNetInterfaceExcludesTunnelInterfaces(t *testing.T) {
	for _, name := range []string{"tailscale0", "tun0", "wg0", "wireguard0", "ipsec0", "gre0", "gretap0", "ipip0", "sit0", "ip6tnl0", "zerotier0"} {
		if shouldIncludeNetInterface(name, nil) {
			t.Fatalf("%s should be excluded", name)
		}
	}
}

func TestShouldIncludeNetInterfaceAllowsPhysicalInterface(t *testing.T) {
	if !shouldIncludeNetInterface("eth0", nil) {
		t.Fatal("eth0 should be included")
	}
}

func TestMemoryUsedMBFromKBUsesMemAvailable(t *testing.T) {
	if got := memoryUsedMBFromKB(8*1024, 3*1024, 0, 0, 0); got != 5 {
		t.Fatalf("got %d, want 5", got)
	}
}

func TestMemoryUsedMBFromKBFallsBackToFreeBuffersCached(t *testing.T) {
	if got := memoryUsedMBFromKB(8*1024, 0, 1*1024, 2*1024, 3*1024); got != 2 {
		t.Fatalf("got %d, want 2", got)
	}
}

func TestMemoryUsedMBFromKBClampsNegativeUsage(t *testing.T) {
	if got := memoryUsedMBFromKB(3*1024, 8*1024, 0, 0, 0); got != 0 {
		t.Fatalf("got %d, want 0", got)
	}
}

func TestSwapUsedMBFromKB(t *testing.T) {
	if got := swapUsedMBFromKB(4*1024, 1*1024); got != 3 {
		t.Fatalf("got %d, want 3", got)
	}
}

func TestSwapUsedMBFromKBClampsNegativeUsage(t *testing.T) {
	if got := swapUsedMBFromKB(1*1024, 4*1024); got != 0 {
		t.Fatalf("got %d, want 0", got)
	}
}
