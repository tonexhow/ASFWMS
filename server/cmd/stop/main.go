// ASFWMS stop binary: no shell required on Windows.
package main

import (
	"asfwms.local/scheduler/internal/runtimecontrol"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func main() {
	root, e := runtimecontrol.DiscoverRoot()
	if e != nil {
		return
	}
	dir := filepath.Join(root, "logs")
	_ = os.MkdirAll(dir, 0700)
	log, e := os.OpenFile(filepath.Join(dir, "stop-actions.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if e != nil {
		return
	}
	defer log.Close()
	stamp := time.Now().Format(time.RFC3339)
	if e = runtimecontrol.SendStop(root); e != nil {
		fmt.Fprintf(log, "%s - Stop failed: %v\n", stamp, e)
		return
	}
	fmt.Fprintf(log, "%s - Graceful stop requested\n", stamp)
}
