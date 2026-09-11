// Package native loads the actual C and Rust shared libraries at startup.
package native

import "calculator/internal/calculator"

// Library must remain open until all native calls have completed.
type Library interface {
	calculator.Native
	Close() error
}
