package plugin

import (
	"fmt"
)

// LoadNativePlugin attempts to load a Go plugin (.so file) and return its
// exported symbols.  Native Go plugins require CGO_ENABLED=1 and matching
// compiler versions between the host binary and the plugin.
//
// This feature is deferred to v0.2.  In v0.1, LoadNativePlugin returns a
// descriptive error explaining the limitation.  The safeCall wrapper for
// recover()-based panic isolation is still functional and exported for
// testing.
func LoadNativePlugin(path string) (any, error) {
	return nil, fmt.Errorf(
		"plugin: native Go plugins (.so) are not yet supported in v0.1; " +
			"planned for v0.2 (requires CGO_ENABLED=1 and matching compiler versions)",
	)
}

// safeCall executes fn inside a recover() guard.  If fn panics, the panic
// value is captured and returned as a descriptive error.  This isolates the
// caller from misbehaving plugin code.
//
// Example:
//
//	err := safeCall(func() error {
//	    return untrustedPlugin.DoSomething()
//	})
func safeCall(fn func() error) error {
	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("plugin panic: %v", r)
			}
		}()
		err = fn()
	}()
	return err
}
