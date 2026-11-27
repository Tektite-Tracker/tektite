// Package a is a fixture.
package a

// Exported has a docblock.
func Exported() {}

// Thing has a docblock.
type Thing struct {
	// Name has a field docblock.
	Name string
	Age  int // want `comment must be`
}

//go:noinline
func Directive() {}

func Body() {
	// want `comment must be`
	x := 1
	_ = x // want `comment must be`

	// why: the lease must outlive the heartbeat interval
	y := 2
	_ = y

	_ = 3 //nolint:errcheck
}
