package metrics

import (
	"path/filepath"
	"runtime"
	"testing"
)

func testdataDir() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "testdata")
}

// --- Parse /proc/self/status ---

// AC-CSM-001: Parse VmRSS from /proc/self/status
func TestAC_CSM001_ParseProcStatusVmRSS(t *testing.T) {
	src := &ProcfsSource{procPath: testdataDir()}
	info, err := src.MemoryInfo()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// VmRSS: 32768 kB = 32768 * 1024 = 33554432 bytes
	expected := uint64(32768 * 1024)
	if info.RSS != expected {
		t.Errorf("expected RSS=%d, got %d", expected, info.RSS)
	}
}

// AC-CSM-001: Parse VmSize from /proc/self/status
func TestAC_CSM001_ParseProcStatusVmSize(t *testing.T) {
	src := &ProcfsSource{procPath: testdataDir()}
	info, err := src.MemoryInfo()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// VmSize: 262144 kB = 262144 * 1024 = 268435456 bytes
	expected := uint64(262144 * 1024)
	if info.VMS != expected {
		t.Errorf("expected VMS=%d, got %d", expected, info.VMS)
	}
}

// --- Parse /proc/net/dev ---

// AC-CSM-002: Parse per-interface receive bytes from /proc/net/dev
func TestAC_CSM002_ParseNetDevReceiveBytes(t *testing.T) {
	src := &ProcfsSource{procPath: testdataDir()}
	stats, err := src.NetworkStats()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := make(map[string]NetStat)
	for _, s := range stats {
		found[s.Interface] = s
	}

	eth0, ok := found["eth0"]
	if !ok {
		t.Fatal("expected eth0 interface in results")
	}
	if eth0.RxBytes != 98765432 {
		t.Errorf("expected eth0 RxBytes=98765432, got %d", eth0.RxBytes)
	}
}

// AC-CSM-002: Parse per-interface transmit bytes from /proc/net/dev
func TestAC_CSM002_ParseNetDevTransmitBytes(t *testing.T) {
	src := &ProcfsSource{procPath: testdataDir()}
	stats, err := src.NetworkStats()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := make(map[string]NetStat)
	for _, s := range stats {
		found[s.Interface] = s
	}

	eth0 := found["eth0"]
	if eth0.TxBytes != 45678901 {
		t.Errorf("expected eth0 TxBytes=45678901, got %d", eth0.TxBytes)
	}
}

// AC-CSM-002: Parse per-interface receive and transmit packets
func TestAC_CSM002_ParseNetDevPackets(t *testing.T) {
	src := &ProcfsSource{procPath: testdataDir()}
	stats, err := src.NetworkStats()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := make(map[string]NetStat)
	for _, s := range stats {
		found[s.Interface] = s
	}

	eth0 := found["eth0"]
	if eth0.RxPackets != 654321 {
		t.Errorf("expected eth0 RxPackets=654321, got %d", eth0.RxPackets)
	}
	if eth0.TxPackets != 321098 {
		t.Errorf("expected eth0 TxPackets=321098, got %d", eth0.TxPackets)
	}

	lo := found["lo"]
	if lo.RxBytes != 1234567 {
		t.Errorf("expected lo RxBytes=1234567, got %d", lo.RxBytes)
	}
}

// --- Parse cgroup memory limit ---

// AC-CSM-003: Parse cgroup v2 memory.max (numeric limit)
func TestAC_CSM003_ParseCgroupV2MemoryMax(t *testing.T) {
	src := &ProcfsSource{
		procPath:   testdataDir(),
		cgroupPath: testdataDir(),
	}
	// We need a directory structure: cgroupPath/memory.max
	// Our testdata has cgroup_memory_max file. We'll test parseCgroupMemoryLimit directly.
	limit, available, err := parseCgroupMemoryLimit(
		filepath.Join(testdataDir(), "cgroup_memory_max"),
		filepath.Join(testdataDir(), "nonexistent"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if limit != 536870912 {
		t.Errorf("expected limit=536870912, got %d", limit)
	}
	if !available {
		t.Error("expected limit to be available")
	}
}

// AC-CSM-003: Parse cgroup v2 memory.max with "max" (unlimited)
func TestAC_CSM003_ParseCgroupV2MemoryMaxUnlimited(t *testing.T) {
	limit, available, err := parseCgroupMemoryLimit(
		filepath.Join(testdataDir(), "cgroup_memory_max_unlimited"),
		filepath.Join(testdataDir(), "nonexistent"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if limit != -1 {
		t.Errorf("expected limit=-1 for 'max', got %d", limit)
	}
	if !available {
		t.Error("expected limit to be available (even if unlimited)")
	}
}

// AC-CSM-003: Parse cgroup v1 fallback
func TestAC_CSM003_ParseCgroupV1Fallback(t *testing.T) {
	limit, available, err := parseCgroupMemoryLimit(
		filepath.Join(testdataDir(), "nonexistent"),
		filepath.Join(testdataDir(), "cgroup_v1_memory_limit"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if limit != 1073741824 {
		t.Errorf("expected limit=1073741824, got %d", limit)
	}
	if !available {
		t.Error("expected limit to be available via v1 fallback")
	}
}

// AC-CSM-003: No cgroup files returns not available
func TestAC_CSM003_NoCgroupFiles(t *testing.T) {
	_, available, err := parseCgroupMemoryLimit(
		filepath.Join(testdataDir(), "nonexistent_v2"),
		filepath.Join(testdataDir(), "nonexistent_v1"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if available {
		t.Error("expected limit not available when no cgroup files exist")
	}
}

// --- CPU usage ---

// AC-CSM-004: CPUUsage returns non-negative values
func TestAC_CSM004_CPUUsageNonNegative(t *testing.T) {
	src := &ProcfsSource{}
	userSec, sysSec, err := src.CPUUsage()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if userSec < 0 {
		t.Errorf("expected userSec >= 0, got %f", userSec)
	}
	if sysSec < 0 {
		t.Errorf("expected sysSec >= 0, got %f", sysSec)
	}
}

// --- Disk usage ---

// AC-CSM-005: DiskUsage returns valid values for root path
func TestAC_CSM005_DiskUsageValidValues(t *testing.T) {
	src := &ProcfsSource{}
	info, err := src.DiskUsage("/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Total == 0 {
		t.Error("expected Total > 0")
	}
	if info.Free > info.Total {
		t.Errorf("Free (%d) should not exceed Total (%d)", info.Free, info.Total)
	}
	if info.Used != info.Total-info.Free {
		t.Errorf("Used (%d) should equal Total-Free (%d)", info.Used, info.Total-info.Free)
	}
}

// --- MemInfo struct fields ---

// AC-CSM-001: MemInfo has LimitAvailable field
func TestAC_CSM001_MemInfoLimitAvailable(t *testing.T) {
	info := &MemInfo{
		RSS:            100,
		VMS:            200,
		Limit:          300,
		LimitAvailable: true,
	}
	if !info.LimitAvailable {
		t.Error("expected LimitAvailable=true")
	}
}
