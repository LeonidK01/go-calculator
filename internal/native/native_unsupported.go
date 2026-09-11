//go:build !linux || !cgo

package native

import "fmt"

// Open fails explicitly on unsupported builds; there is no simulated native backend.
func Open(_, _ string) (Library, error) {
	return nil, fmt.Errorf("calculator_server requires Linux and CGO_ENABLED=1; use Docker on Windows/macOS")
}
