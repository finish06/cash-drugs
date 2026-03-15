package metrics

import (
	"log/slog"
	"sync"
	"time"
)

// SystemCollector periodically collects container system metrics.
type SystemCollector struct {
	source   SystemSource
	metrics  *Metrics
	interval time.Duration
	diskPath string
	stopCh   chan struct{}
	done     chan struct{}
	stopOnce sync.Once
}

// NewSystemCollector creates a new background system metrics collector.
func NewSystemCollector(source SystemSource, m *Metrics, interval time.Duration, diskPath string) *SystemCollector {
	return &SystemCollector{
		source:   source,
		metrics:  m,
		interval: interval,
		diskPath: diskPath,
		stopCh:   make(chan struct{}),
		done:     make(chan struct{}),
	}
}

// Start begins the background collection loop.
func (c *SystemCollector) Start() {
	go func() {
		defer close(c.done)

		// Collect once immediately
		c.collect()

		ticker := time.NewTicker(c.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				c.collect()
			case <-c.stopCh:
				return
			}
		}
	}()
}

// Stop signals the collector to stop and waits for the goroutine to exit.
func (c *SystemCollector) Stop() {
	c.stopOnce.Do(func() { close(c.stopCh) })
	<-c.done
}

func (c *SystemCollector) collect() {
	defer func() {
		if r := recover(); r != nil {
			slog.Debug("system metrics collect panic recovered", "component", "metrics", "panic", r)
		}
	}()

	// CPU
	userSec, sysSec, err := c.source.CPUUsage()
	if err != nil {
		slog.Debug("system metrics cpu usage failed", "component", "metrics", "error", err)
	} else {
		c.metrics.ContainerCPUUsage.Set(userSec + sysSec)
	}

	// CPU cores
	c.metrics.ContainerCPUCores.Set(float64(NumCPU()))

	// Memory
	memInfo, err := c.source.MemoryInfo()
	if err != nil {
		slog.Debug("system metrics memory info failed", "component", "metrics", "error", err)
	} else if memInfo != nil {
		c.metrics.ContainerMemoryRSS.Set(float64(memInfo.RSS))
		c.metrics.ContainerMemoryVMS.Set(float64(memInfo.VMS))
		c.metrics.ContainerMemoryLimit.Set(float64(memInfo.Limit))

		// Only set usage ratio if limit is positive (not unlimited)
		if memInfo.Limit > 0 {
			c.metrics.ContainerMemoryUsageRatio.Set(float64(memInfo.RSS) / float64(memInfo.Limit))
		}
	}

	// Disk
	diskInfo, err := c.source.DiskUsage(c.diskPath)
	if err != nil {
		slog.Debug("system metrics disk usage failed", "component", "metrics", "error", err)
	} else {
		c.metrics.ContainerDiskTotal.Set(float64(diskInfo.Total))
		c.metrics.ContainerDiskFree.Set(float64(diskInfo.Free))
		c.metrics.ContainerDiskUsed.Set(float64(diskInfo.Used))
	}

	// Network
	netStats, err := c.source.NetworkStats()
	if err != nil {
		slog.Debug("system metrics network stats failed", "component", "metrics", "error", err)
	} else {
		// Reset gauge vecs before setting to clear stale interface labels
		c.metrics.ContainerNetworkReceiveBytes.Reset()
		c.metrics.ContainerNetworkTransmitBytes.Reset()
		c.metrics.ContainerNetworkReceivePackets.Reset()
		c.metrics.ContainerNetworkTransmitPackets.Reset()

		for _, s := range netStats {
			c.metrics.ContainerNetworkReceiveBytes.WithLabelValues(s.Interface).Set(float64(s.RxBytes))
			c.metrics.ContainerNetworkTransmitBytes.WithLabelValues(s.Interface).Set(float64(s.TxBytes))
			c.metrics.ContainerNetworkReceivePackets.WithLabelValues(s.Interface).Set(float64(s.RxPackets))
			c.metrics.ContainerNetworkTransmitPackets.WithLabelValues(s.Interface).Set(float64(s.TxPackets))
		}
	}
}
