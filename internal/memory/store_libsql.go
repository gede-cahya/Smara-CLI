// Package memory: libSQL driver registration.
//
// This file isolates the libSQL (Turso) `database/sql` driver registration
// behind a blank import so it can be disabled via build tag in the future
// (e.g. `//go:build !no_libsql`) without touching the rest of the package.
//
// The libsql-client-go driver registers itself under the name "libsql"
// during init(). NewSQLiteStoreWithDSN routes to this driver when the DSN
// is recognized as a libSQL embedded-replica connection (see detectDialect
// in store.go).
package memory

import (
	// Driver registration only — no symbols are referenced directly.
	_ "github.com/tursodatabase/libsql-client-go/libsql"
)
