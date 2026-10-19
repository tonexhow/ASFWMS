//go:build windows

package httpapi

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func platformMetrics(ctx context.Context, dbPath string, m *machineStats) {
	drive := strings.TrimSuffix(filepath.VolumeName(dbPath), ":")
	sub, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	// Read-only local CIM queries. No external commands or remote PowerShell.
	script := `$ErrorActionPreference='SilentlyContinue';` +
		`$c=Get-CimInstance Win32_Processor | Select-Object -First 1;` +
		`$o=Get-CimInstance Win32_OperatingSystem;` +
		`$g=@(Get-CimInstance Win32_VideoController | Select-Object -ExpandProperty Name);` +
		`$v=Get-CimInstance Win32_LogicalDisk -Filter "DeviceID='` + drive + `:'";` +
		`[PSCustomObject]@{ cpu=$c.Name; usage=$c.LoadPercentage; ram=$o.TotalVisibleMemorySize; free=$o.FreePhysicalMemory; gpu=$g; disk=$v.Size; diskfree=$v.FreeSpace } | ConvertTo-Json -Compress -Depth 3`
	cmd := exec.CommandContext(sub, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	raw, err := cmd.Output()
	if err != nil {
		m.Note = "Windows hardware queries unavailable or timed out."
		return
	}
	var x struct {
		CPU      string   `json:"cpu"`
		Usage    float64  `json:"usage"`
		RAM      uint64   `json:"ram"`
		Free     uint64   `json:"free"`
		GPU      []string `json:"gpu"`
		Disk     uint64   `json:"disk"`
		DiskFree uint64   `json:"diskfree"`
	}
	// ConvertTo-Json makes a scalar GPU string for single devices; tolerate both.
	var generic map[string]json.RawMessage
	if json.Unmarshal(raw, &generic) != nil {
		return
	}
	_ = json.Unmarshal(generic["cpu"], &x.CPU)
	_ = json.Unmarshal(generic["usage"], &x.Usage)
	_ = json.Unmarshal(generic["ram"], &x.RAM)
	_ = json.Unmarshal(generic["free"], &x.Free)
	_ = json.Unmarshal(generic["disk"], &x.Disk)
	_ = json.Unmarshal(generic["diskfree"], &x.DiskFree)
	if json.Unmarshal(generic["gpu"], &x.GPU) != nil {
		var single string
		if json.Unmarshal(generic["gpu"], &single) == nil && single != "" {
			x.GPU = []string{single}
		}
	}
	if x.CPU != "" {
		m.CPUModel = x.CPU
	}
	m.CPUUsage = x.Usage
	m.RAMTotal = x.RAM * 1024
	m.RAMFree = x.Free * 1024
	m.GPU = x.GPU
	m.DiskTotal = x.Disk
	m.DiskFree = x.DiskFree
	m.DiskLabel = filepath.VolumeName(dbPath)
}
