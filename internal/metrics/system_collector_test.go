package metrics_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/finish06/cash-drugs/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// --- Mock SystemSource ---

type mockSystemSource struct {
	cpuUserSec float64
	cpuSysSec  float64
	cpuErr     error

	memInfo *metrics.MemInfo
	memErr  error

	diskInfo *metrics.DiskInfo
	diskErr  error

	netStats []metrics.NetStat
	netErr   error
}

func (m *mockSystemSource) CPUUsage() (float64, float64, error) {
	return m.cpuUserSec, m.cpuSysSec, m.cpuErr
}

func (m *mockSystemSource) MemoryInfo() (*metrics.MemInfo, error) {
	return m.memInfo, m.memErr
}

func (m *mockSystemSource) DiskUsage(path string) (*metrics.DiskInfo, error) {
	return m.diskInfo, m.diskErr
}

func (m *mockSystemSource) NetworkStats() ([]metrics.NetStat, error) {
	return m.netStats, m.netErr
}

// --- Lifecycle Tests ---

// AC-CSM-006: SystemCollector starts and stops cleanly
func TestAC_CSM006_SystemCollectorStartStop(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	src := &mockSystemSource{
		memInfo:  &metrics.MemInfo{RSS: 1000, VMS: 2000, Limit: -1},
		diskInfo: &metrics.DiskInfo{Total: 100, Free: 50, Used: 50},
	}

	collector := metrics.NewSystemCollector(src, m, 1*time.Hour, "/")
	collector.Start()

	done := make(chan struct{})
	go func() {
		collector.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("collector.Stop() did not return within 2 seconds")
	}
}

// AC-CSM-006: Double Stop does not panic
func TestAC_CSM006_SystemCollectorDoubleStopNoPanic(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	src := &mockSystemSource{
		memInfo:  &metrics.MemInfo{RSS: 1000, VMS: 2000, Limit: -1},
		diskInfo: &metrics.DiskInfo{Total: 100, Free: 50, Used: 50},
	}

	collector := metrics.NewSystemCollector(src, m, 1*time.Hour, "/")
	collector.Start()
	collector.Stop()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("double Stop() panicked: %v", r)
		}
	}()
	collector.Stop()
}

// --- Metrics Collection Tests ---

// AC-CSM-007: Metrics are set after collection runs
func TestAC_CSM007_MetricsSetAfterCollect(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	src := &mockSystemSource{
		cpuUserSec: 1.5,
		cpuSysSec:  0.5,
		memInfo: &metrics.MemInfo{
			RSS:            33554432,
			VMS:            268435456,
			Limit:          536870912,
			LimitAvailable: true,
		},
		diskInfo: &metrics.DiskInfo{
			Total: 107374182400,
			Free:  53687091200,
			Used:  53687091200,
		},
		netStats: []metrics.NetStat{
			{Interface: "eth0", RxBytes: 98765432, TxBytes: 45678901, RxPackets: 654321, TxPackets: 321098},
		},
	}

	collector := metrics.NewSystemCollector(src, m, 1*time.Hour, "/")
	collector.Start()
	time.Sleep(100 * time.Millisecond)
	collector.Stop()

	// CPU
	cpuVal := testutil.ToFloat64(m.ContainerCPUUsage)
	if cpuVal != 2.0 { // 1.5 + 0.5
		t.Errorf("expected CPU usage=2.0, got %f", cpuVal)
	}

	// Memory RSS
	rssVal := testutil.ToFloat64(m.ContainerMemoryRSS)
	if rssVal != 33554432 {
		t.Errorf("expected MemoryRSS=33554432, got %f", rssVal)
	}

	// Memory VMS
	vmsVal := testutil.ToFloat64(m.ContainerMemoryVMS)
	if vmsVal != 268435456 {
		t.Errorf("expected MemoryVMS=268435456, got %f", vmsVal)
	}

	// Memory Limit
	limitVal := testutil.ToFloat64(m.ContainerMemoryLimit)
	if limitVal != 536870912 {
		t.Errorf("expected MemoryLimit=536870912, got %f", limitVal)
	}

	// Memory Usage Ratio (RSS / Limit)
	ratioVal := testutil.ToFloat64(m.ContainerMemoryUsageRatio)
	expectedRatio := float64(33554432) / float64(536870912) // ~0.0625
	if ratioVal < expectedRatio-0.001 || ratioVal > expectedRatio+0.001 {
		t.Errorf("expected MemoryUsageRatio~%f, got %f", expectedRatio, ratioVal)
	}

	// Disk
	diskTotal := testutil.ToFloat64(m.ContainerDiskTotal)
	if diskTotal != 107374182400 {
		t.Errorf("expected DiskTotal=107374182400, got %f", diskTotal)
	}

	diskFree := testutil.ToFloat64(m.ContainerDiskFree)
	if diskFree != 53687091200 {
		t.Errorf("expected DiskFree=53687091200, got %f", diskFree)
	}

	diskUsed := testutil.ToFloat64(m.ContainerDiskUsed)
	if diskUsed != 53687091200 {
		t.Errorf("expected DiskUsed=53687091200, got %f", diskUsed)
	}

	// Network
	rxBytes := testutil.ToFloat64(m.ContainerNetworkReceiveBytes.WithLabelValues("eth0"))
	if rxBytes != 98765432 {
		t.Errorf("expected eth0 RxBytes=98765432, got %f", rxBytes)
	}

	txBytes := testutil.ToFloat64(m.ContainerNetworkTransmitBytes.WithLabelValues("eth0"))
	if txBytes != 45678901 {
		t.Errorf("expected eth0 TxBytes=45678901, got %f", txBytes)
	}

	rxPkts := testutil.ToFloat64(m.ContainerNetworkReceivePackets.WithLabelValues("eth0"))
	if rxPkts != 654321 {
		t.Errorf("expected eth0 RxPackets=654321, got %f", rxPkts)
	}

	txPkts := testutil.ToFloat64(m.ContainerNetworkTransmitPackets.WithLabelValues("eth0"))
	if txPkts != 321098 {
		t.Errorf("expected eth0 TxPackets=321098, got %f", txPkts)
	}
}

// AC-CSM-008: Memory usage ratio not set when limit is -1 (unlimited)
func TestAC_CSM008_MemoryUsageRatioNotSetWhenUnlimited(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	src := &mockSystemSource{
		memInfo: &metrics.MemInfo{
			RSS:            33554432,
			VMS:            268435456,
			Limit:          -1,
			LimitAvailable: true,
		},
		diskInfo: &metrics.DiskInfo{Total: 100, Free: 50, Used: 50},
	}

	collector := metrics.NewSystemCollector(src, m, 1*time.Hour, "/")
	collector.Start()
	time.Sleep(100 * time.Millisecond)
	collector.Stop()

	// Memory usage ratio should remain at 0 (not set) when limit is -1
	ratioVal := testutil.ToFloat64(m.ContainerMemoryUsageRatio)
	if ratioVal != 0 {
		t.Errorf("expected MemoryUsageRatio=0 when limit=-1, got %f", ratioVal)
	}
}

// AC-CSM-009: Panic in collect is recovered, collector continues
func TestAC_CSM009_PanicRecovery(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	// A source where memInfo is nil to trigger a nil pointer if not handled
	src := &mockSystemSource{
		memInfo:  nil,
		memErr:   nil,
		diskInfo: &metrics.DiskInfo{Total: 100, Free: 50, Used: 50},
	}

	collector := metrics.NewSystemCollector(src, m, 50*time.Millisecond, "/")
	collector.Start()
	time.Sleep(200 * time.Millisecond) // Let several ticks pass
	collector.Stop()

	// If we got here, the panic was recovered
}

// AC-CSM-010: All container metric names use cashdrugs_container_ prefix
func TestAC_CSM010_ContainerMetricPrefix(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	// Initialize some gauge vecs to ensure they show up
	m.ContainerNetworkReceiveBytes.WithLabelValues("test").Set(1)
	m.ContainerNetworkTransmitBytes.WithLabelValues("test").Set(1)
	m.ContainerNetworkReceivePackets.WithLabelValues("test").Set(1)
	m.ContainerNetworkTransmitPackets.WithLabelValues("test").Set(1)

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	containerMetrics := 0
	for _, f := range families {
		if strings.HasPrefix(*f.Name, "cashdrugs_container_") {
			containerMetrics++
		}
	}

	// We expect 13 container metrics (9 gauges + 4 gauge vecs)
	if containerMetrics < 13 {
		t.Errorf("expected at least 13 container metrics with cashdrugs_container_ prefix, found %d", containerMetrics)
	}
}

// AC-CSM: collect handles CPU error gracefully
func TestCollect_CPUError(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	src := &mockSystemSource{
		cpuErr:   fmt.Errorf("cpu error"),
		memInfo:  &metrics.MemInfo{RSS: 1000, VMS: 2000, Limit: -1},
		diskInfo: &metrics.DiskInfo{Total: 100, Free: 50, Used: 50},
	}

	collector := metrics.NewSystemCollector(src, m, 1*time.Hour, "/")
	collector.Start()
	time.Sleep(100 * time.Millisecond)
	collector.Stop()

	// CPU value should remain at 0 (error path)
	cpuVal := testutil.ToFloat64(m.ContainerCPUUsage)
	if cpuVal != 0 {
		t.Errorf("expected CPU usage=0 on error, got %f", cpuVal)
	}
	// Memory should still be collected
	rssVal := testutil.ToFloat64(m.ContainerMemoryRSS)
	if rssVal != 1000 {
		t.Errorf("expected RSS=1000, got %f", rssVal)
	}
}

// AC-CSM: collect handles disk error gracefully
func TestCollect_DiskError(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	src := &mockSystemSource{
		memInfo: &metrics.MemInfo{RSS: 1000, VMS: 2000, Limit: -1},
		diskErr: fmt.Errorf("disk error"),
	}

	collector := metrics.NewSystemCollector(src, m, 1*time.Hour, "/")
	collector.Start()
	time.Sleep(100 * time.Millisecond)
	collector.Stop()

	// Disk values should remain at 0
	diskTotal := testutil.ToFloat64(m.ContainerDiskTotal)
	if diskTotal != 0 {
		t.Errorf("expected DiskTotal=0 on error, got %f", diskTotal)
	}
}

// AC-CSM: collect handles network error gracefully
func TestCollect_NetworkError(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	src := &mockSystemSource{
		memInfo:  &metrics.MemInfo{RSS: 1000, VMS: 2000, Limit: -1},
		diskInfo: &metrics.DiskInfo{Total: 100, Free: 50, Used: 50},
		netErr:   fmt.Errorf("network error"),
	}

	collector := metrics.NewSystemCollector(src, m, 1*time.Hour, "/")
	collector.Start()
	time.Sleep(100 * time.Millisecond)
	collector.Stop()

	// Should not panic, memory and disk should still be collected
	rssVal := testutil.ToFloat64(m.ContainerMemoryRSS)
	if rssVal != 1000 {
		t.Errorf("expected RSS=1000, got %f", rssVal)
	}
}

// AC-CSM: collect handles memory error gracefully
func TestCollect_MemoryError(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	src := &mockSystemSource{
		cpuUserSec: 1.0,
		cpuSysSec:  0.5,
		memErr:     fmt.Errorf("memory error"),
		diskInfo:   &metrics.DiskInfo{Total: 100, Free: 50, Used: 50},
	}

	collector := metrics.NewSystemCollector(src, m, 1*time.Hour, "/")
	collector.Start()
	time.Sleep(100 * time.Millisecond)
	collector.Stop()

	// CPU should still be collected
	cpuVal := testutil.ToFloat64(m.ContainerCPUUsage)
	if cpuVal != 1.5 {
		t.Errorf("expected CPU=1.5, got %f", cpuVal)
	}
	// Memory should remain at 0
	rssVal := testutil.ToFloat64(m.ContainerMemoryRSS)
	if rssVal != 0 {
		t.Errorf("expected RSS=0 on memory error, got %f", rssVal)
	}
}

// AC-CSM: collect handles memory limit=0 (no usage ratio set)
func TestCollect_MemoryLimitZero(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	src := &mockSystemSource{
		memInfo: &metrics.MemInfo{
			RSS:            33554432,
			VMS:            268435456,
			Limit:          0, // no limit
			LimitAvailable: false,
		},
		diskInfo: &metrics.DiskInfo{Total: 100, Free: 50, Used: 50},
	}

	collector := metrics.NewSystemCollector(src, m, 1*time.Hour, "/")
	collector.Start()
	time.Sleep(100 * time.Millisecond)
	collector.Stop()

	ratioVal := testutil.ToFloat64(m.ContainerMemoryUsageRatio)
	if ratioVal != 0 {
		t.Errorf("expected MemoryUsageRatio=0 when limit=0, got %f", ratioVal)
	}
}

// AC-CSM-011: CPU cores metric is set
func TestAC_CSM011_CPUCoresMetric(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	src := &mockSystemSource{
		memInfo:  &metrics.MemInfo{RSS: 1000, VMS: 2000, Limit: -1},
		diskInfo: &metrics.DiskInfo{Total: 100, Free: 50, Used: 50},
	}

	collector := metrics.NewSystemCollector(src, m, 1*time.Hour, "/")
	collector.Start()
	time.Sleep(100 * time.Millisecond)
	collector.Stop()

	cores := testutil.ToFloat64(m.ContainerCPUCores)
	if cores <= 0 {
		t.Errorf("expected CPUCores > 0, got %f", cores)
	}
}
