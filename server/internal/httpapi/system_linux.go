//go:build linux

package httpapi

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func linuxCPU() (uint64, uint64) {
	data, e := os.ReadFile("/proc/stat")
	if e != nil {
		return 0, 0
	}
	line := strings.SplitN(string(data), "\n", 2)[0]
	fields := strings.Fields(line)
	var total, idle uint64
	for i, part := range fields {
		if i == 0 {
			continue
		}
		v, _ := strconv.ParseUint(part, 10, 64)
		total += v
		if i == 4 || i == 5 {
			idle += v
		}
	}
	return total, idle
}
func platformMetrics(ctx context.Context, dbPath string, m *machineStats) {
	data, e := os.ReadFile("/proc/cpuinfo")
	if e == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "model name") {
				m.CPUModel = strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
				break
			}
		}
	}
	a, b := linuxCPU()
	select {
	case <-ctx.Done():
		return
	case <-time.After(125 * time.Millisecond):
	}
	c, d := linuxCPU()
	if c > a {
		m.CPUUsage = 100 * (1 - float64(d-b)/float64(c-a))
	}
	data, e = os.ReadFile("/proc/meminfo")
	if e == nil {
		for _, line := range strings.Split(string(data), "\n") {
			words := strings.Fields(line)
			if len(words) < 2 {
				continue
			}
			n, _ := strconv.ParseUint(words[1], 10, 64)
			switch words[0] {
			case "MemTotal:":
				m.RAMTotal = n * 1024
			case "MemAvailable:":
				m.RAMFree = n * 1024
			}
		}
	}
	var s syscall.Statfs_t
	if syscall.Statfs(filepath.Dir(dbPath), &s) == nil {
		m.DiskTotal = s.Blocks * uint64(s.Bsize)
		m.DiskFree = s.Bavail * uint64(s.Bsize)
		m.DiskLabel = filepath.Dir(dbPath)
	}
	for _, base := range []string{"/sys/class/drm", "/sys/class/video4linux"} {
		entries, e := os.ReadDir(base)
		if e != nil {
			continue
		}
		for _, entry := range entries {
			if !strings.HasPrefix(entry.Name(), "card") || strings.Contains(entry.Name(), "-") {
				continue
			}
			vendor, e := os.ReadFile(filepath.Join(base, entry.Name(), "device/vendor"))
			if e == nil {
				m.GPU = append(m.GPU, "Graphics device "+strings.TrimSpace(string(vendor)))
			}
		}
	}
	if len(m.GPU) == 0 {
		m.Note = "GPU utilization is not exposed by this host."
	}
}
