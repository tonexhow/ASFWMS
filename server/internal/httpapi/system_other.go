//go:build !linux && !windows

package httpapi

import "context"

func platformMetrics(ctx context.Context, dbPath string, m *machineStats) {
	m.Note = "Detailed hardware metrics are currently supported on Windows and Linux."
}
