package metrics

// SystemSource abstracts system-level metrics collection for testability.
type SystemSource interface {
	// CPUUsage returns user and system CPU time in seconds.
	CPUUsage() (userSec, sysSec float64, err error)

	// MemoryInfo returns memory information for the current process.
	MemoryInfo() (*MemInfo, error)

	// DiskUsage returns disk usage information for the given path.
	DiskUsage(path string) (*DiskInfo, error)

	// NetworkStats returns per-interface network statistics.
	NetworkStats() ([]NetStat, error)
}

// MemInfo holds memory information for the current process.
type MemInfo struct {
	RSS            uint64 // Resident set size in bytes
	VMS            uint64 // Virtual memory size in bytes
	Limit          int64  // Memory limit in bytes (-1 if unlimited)
	LimitAvailable bool   // Whether a memory limit was found
}

// DiskInfo holds disk usage information.
type DiskInfo struct {
	Total uint64 // Total disk space in bytes
	Free  uint64 // Free disk space in bytes
	Used  uint64 // Used disk space in bytes
}

// NetStat holds per-interface network statistics.
type NetStat struct {
	Interface string
	RxBytes   uint64
	TxBytes   uint64
	RxPackets uint64
	TxPackets uint64
}
