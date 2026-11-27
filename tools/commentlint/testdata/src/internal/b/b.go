package b

// Exported is documented but internal packages take no docblocks. // want `comment must be`
func Exported() {}

//go:noinline
func Directive() {}

// why: internal code may still explain a decision
var Reason = 1
