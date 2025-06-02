package builtins

// registerArray is currently empty since Array prototype methods are handled directly in the VM
// The Array constructor itself is already registered in builtins.go
func registerArray() {
	// Array prototype methods like concat, push, pop are now handled directly
	// in the VM's prototype system (see pkg/vm/prototypes.go)
}
