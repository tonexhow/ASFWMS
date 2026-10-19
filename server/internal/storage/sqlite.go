//go:build !sqlite_native

package storage

import (
	"context"
	"modernc.org/sqlite"
)

// Apply connection-local safety settings even if database/sql replaces a connection.
func init() {
	sqlite.RegisterConnectionHook(func(c sqlite.ExecQuerierContext, _ string) error {
		if _, e := c.ExecContext(context.Background(), "PRAGMA foreign_keys=ON", nil); e != nil {
			return e
		}
		_, e := c.ExecContext(context.Background(), "PRAGMA busy_timeout=5000", nil)
		return e
	})
}
