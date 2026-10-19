package httpapi

import (
	"context"
	"net"
	"runtime"
	"time"
)

type machineStats struct {
	CPUModel  string   `json:"cpu_model"`
	CPUUsage  float64  `json:"cpu_usage_percent"`
	Cores     int      `json:"cores"`
	GPU       []string `json:"gpu"`
	RAMTotal  uint64   `json:"ram_total_bytes"`
	RAMFree   uint64   `json:"ram_available_bytes"`
	DiskTotal uint64   `json:"storage_total_bytes"`
	DiskFree  uint64   `json:"storage_free_bytes"`
	DiskLabel string   `json:"storage_label"`
	Note      string   `json:"note"`
}

func readMachine(ctx context.Context, dbPath string) machineStats {
	m := machineStats{Cores: runtime.NumCPU(), GPU: []string{}, CPUModel: "Unavailable", DiskLabel: "Database volume"}
	platformMetrics(ctx, dbPath, &m)
	return m
}
func connectivity(ctx context.Context) map[string]any {
	interfaces, _ := net.Interfaces()
	names := []string{}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp != 0 && iface.Flags&net.FlagLoopback == 0 {
			names = append(names, iface.Name)
		}
	}
	dialer := net.Dialer{Timeout: 1100 * time.Millisecond}
	c, err := dialer.DialContext(ctx, "tcp", "1.1.1.1:443")
	if c != nil {
		_ = c.Close()
	}
	status := "Not verified (network or firewall restrictions)"
	if err == nil {
		status = "Reachable (outbound TCP test)"
	}
	return map[string]any{"status": status, "reachable": err == nil, "probe": "1.1.1.1:443", "interfaces": names}
}
