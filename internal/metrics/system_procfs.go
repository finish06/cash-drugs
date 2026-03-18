package metrics

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)

// ProcfsSource implements SystemSource using procfs and cgroup files.
// Configurable base paths allow testing with fixture files.
type ProcfsSource struct {
	procPath   string // default: "/proc"
	cgroupPath string // default: "/sys/fs/cgroup"
}

// NewProcfsSource creates a ProcfsSource with default paths.
func NewProcfsSource() *ProcfsSource {
	return &ProcfsSource{
		procPath:   "/proc",
		cgroupPath: "/sys/fs/cgroup",
	}
}

// CPUUsage returns user and system CPU time in seconds via syscall.Getrusage.
func (s *ProcfsSource) CPUUsage() (float64, float64, error) {
	var rusage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &rusage); err != nil {
		return 0, 0, fmt.Errorf("getrusage: %w", err)
	}
	userSec := float64(rusage.Utime.Sec) + float64(rusage.Utime.Usec)/1e6
	sysSec := float64(rusage.Stime.Sec) + float64(rusage.Stime.Usec)/1e6
	return userSec, sysSec, nil
}

// resolvePath tries the primary path first; if it doesn't exist, tries altPath.
// This allows tests to use flat testdata files instead of nested /proc/self/status paths.
func resolvePath(primary, alt string) string {
	if _, err := os.Stat(primary); os.IsNotExist(err) {
		if _, err2 := os.Stat(alt); err2 == nil {
			return alt
		}
	}
	return primary
}

// MemoryInfo parses /proc/self/status for VmRSS and VmSize, and reads cgroup memory limit.
func (s *ProcfsSource) MemoryInfo() (*MemInfo, error) {
	statusPath := resolvePath(
		s.procPath+"/self/status",
		s.procPath+"/proc_self_status",
	)

	info := &MemInfo{}

	f, err := os.Open(statusPath)
	if err != nil {
		return nil, fmt.Errorf("open proc status: %w", err)
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "VmRSS:") {
			val, err := parseKBLine(line)
			if err != nil {
				return nil, fmt.Errorf("parse VmRSS: %w", err)
			}
			info.RSS = val * 1024 // Convert kB to bytes
		} else if strings.HasPrefix(line, "VmSize:") {
			val, err := parseKBLine(line)
			if err != nil {
				return nil, fmt.Errorf("parse VmSize: %w", err)
			}
			info.VMS = val * 1024 // Convert kB to bytes
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan proc status: %w", err)
	}

	// Read cgroup memory limit
	v2Path := s.cgroupPath + "/memory.max"
	v1Path := s.cgroupPath + "/memory/memory.limit_in_bytes"
	limit, available, err := parseCgroupMemoryLimit(v2Path, v1Path)
	if err != nil {
		return nil, fmt.Errorf("parse cgroup memory limit: %w", err)
	}
	info.Limit = limit
	info.LimitAvailable = available

	return info, nil
}

// DiskUsage returns disk usage for the given path via syscall.Statfs.
func (s *ProcfsSource) DiskUsage(path string) (*DiskInfo, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return nil, fmt.Errorf("statfs %s: %w", path, err)
	}

	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bavail * uint64(stat.Bsize)
	used := total - free

	return &DiskInfo{
		Total: total,
		Free:  free,
		Used:  used,
	}, nil
}

// NetworkStats parses /proc/net/dev for per-interface statistics.
func (s *ProcfsSource) NetworkStats() ([]NetStat, error) {
	netDevPath := resolvePath(
		s.procPath+"/net/dev",
		s.procPath+"/proc_net_dev",
	)

	f, err := os.Open(netDevPath)
	if err != nil {
		return nil, fmt.Errorf("open net dev: %w", err)
	}
	defer func() { _ = f.Close() }()

	var stats []NetStat
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		// Skip the first two header lines
		if lineNum <= 2 {
			continue
		}

		line := scanner.Text()
		stat, err := parseNetDevLine(line)
		if err != nil {
			continue // Skip unparseable lines
		}
		stats = append(stats, stat)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan net dev: %w", err)
	}

	return stats, nil
}

// NumCPU returns the number of CPUs available.
func NumCPU() int {
	return runtime.NumCPU()
}

// parseKBLine parses a line like "VmRSS:    32768 kB" and returns the value in kB.
func parseKBLine(line string) (uint64, error) {
	// Split on ":" to get the value part
	parts := strings.SplitN(line, ":", 2)
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid line format: %s", line)
	}
	valueStr := strings.TrimSpace(parts[1])
	// Remove "kB" suffix
	valueStr = strings.TrimSuffix(valueStr, " kB")
	valueStr = strings.TrimSpace(valueStr)
	return strconv.ParseUint(valueStr, 10, 64)
}

// parseNetDevLine parses a single line from /proc/net/dev.
func parseNetDevLine(line string) (NetStat, error) {
	// Line format: "  eth0: 98765432  654321    5    2    0     0          0         0 45678901  321098    3    1    0     0       0          0"
	parts := strings.SplitN(line, ":", 2)
	if len(parts) != 2 {
		return NetStat{}, fmt.Errorf("invalid net dev line")
	}

	iface := strings.TrimSpace(parts[0])
	fields := strings.Fields(parts[1])
	if len(fields) < 10 {
		return NetStat{}, fmt.Errorf("not enough fields in net dev line")
	}

	rxBytes, err := strconv.ParseUint(fields[0], 10, 64)
	if err != nil {
		return NetStat{}, fmt.Errorf("parse rx bytes: %w", err)
	}
	rxPackets, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return NetStat{}, fmt.Errorf("parse rx packets: %w", err)
	}
	txBytes, err := strconv.ParseUint(fields[8], 10, 64)
	if err != nil {
		return NetStat{}, fmt.Errorf("parse tx bytes: %w", err)
	}
	txPackets, err := strconv.ParseUint(fields[9], 10, 64)
	if err != nil {
		return NetStat{}, fmt.Errorf("parse tx packets: %w", err)
	}

	return NetStat{
		Interface: iface,
		RxBytes:   rxBytes,
		TxBytes:   txBytes,
		RxPackets: rxPackets,
		TxPackets: txPackets,
	}, nil
}

// parseCgroupMemoryLimit reads the memory limit from cgroup v2 or v1 files.
// Returns the limit in bytes, whether a limit was found, and any error.
// A limit of -1 with available=true means "max" (unlimited).
func parseCgroupMemoryLimit(v2Path, v1Path string) (int64, bool, error) {
	// Try cgroup v2 first
	data, err := os.ReadFile(v2Path)
	if err == nil {
		val := strings.TrimSpace(string(data))
		if val == "max" {
			return -1, true, nil
		}
		limit, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			return 0, false, fmt.Errorf("parse cgroup v2 memory.max: %w", err)
		}
		return limit, true, nil
	}

	// Fall back to cgroup v1
	data, err = os.ReadFile(v1Path)
	if err == nil {
		val := strings.TrimSpace(string(data))
		limit, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			return 0, false, fmt.Errorf("parse cgroup v1 memory.limit_in_bytes: %w", err)
		}
		return limit, true, nil
	}

	// No cgroup files found
	return 0, false, nil
}
