package vm

import "strconv"

// Private names are compiled to keys "K:name", where K identifies the class
// declaration that declares #name. ECMAScript makes each *evaluation* of that
// class declare distinct Private Names (ClassDefinitionEvaluation's
// NewPrivateEnvironment), so a second evaluation's instances must not pass
// the first evaluation's brand checks.
//
// Each class evaluation therefore runs with a PrivateEnv node for K. Closures
// created inside the class body capture the node chain (ClosureObject.
// PrivateEnv); the class evaluation code itself, which runs in the enclosing
// frame, sees it through vm.privateEnvStack (tagged with frame depth like the
// with-object stack). A private key resolves through the innermost node for
// its K: the first evaluation of K keeps the compiled key unchanged, later
// ones get a key suffixed with their evaluation number.

// PrivateEnv is one class evaluation's private environment.
type PrivateEnv struct {
	classID int
	eval    int               // evaluation number of classID, from 1
	keys    map[string]string // compiled key -> this evaluation's key (eval > 1)
	outer   *PrivateEnv
}

type privateEnvEntry struct {
	depth int // vm.frameCount of the frame evaluating the class
	env   *PrivateEnv
}

// currentPrivateEnv is the private environment code in the current frame
// runs with.
func (vm *VM) currentPrivateEnv(closure *ClosureObject) *PrivateEnv {
	if n := len(vm.privateEnvStack); n != 0 {
		top := vm.privateEnvStack[n-1]
		if top.depth == vm.frameCount {
			return top.env
		}
		if top.depth > vm.frameCount {
			vm.trimPrivateEnvStack()
			return vm.currentPrivateEnv(closure)
		}
	}
	if closure != nil {
		return closure.PrivateEnv
	}
	return nil
}

func (vm *VM) trimPrivateEnvStack() {
	n := len(vm.privateEnvStack)
	for n > 0 && vm.privateEnvStack[n-1].depth > vm.frameCount {
		n--
	}
	clear(vm.privateEnvStack[n:])
	vm.privateEnvStack = vm.privateEnvStack[:n]
}

// pushPrivateEnv starts an evaluation of class classID in the current frame.
func (vm *VM) pushPrivateEnv(closure *ClosureObject, classID int) {
	outer := vm.currentPrivateEnv(closure)
	if vm.privateEvalCount == nil {
		vm.privateEvalCount = make(map[int]int)
	}
	vm.privateEvalCount[classID]++
	env := &PrivateEnv{classID: classID, eval: vm.privateEvalCount[classID], outer: outer}
	vm.privateEnvStack = append(vm.privateEnvStack, privateEnvEntry{depth: vm.frameCount, env: env})
}

// popPrivateEnv ends the current frame's innermost class evaluation.
func (vm *VM) popPrivateEnv() {
	vm.trimPrivateEnvStack()
	if n := len(vm.privateEnvStack); n != 0 && vm.privateEnvStack[n-1].depth == vm.frameCount {
		vm.privateEnvStack[n-1] = privateEnvEntry{}
		vm.privateEnvStack = vm.privateEnvStack[:n-1]
	}
}

// resolvePrivateKey maps a compiled private key to the key of the class
// evaluation whose private environment the current code runs in.
func (vm *VM) resolvePrivateKey(closure *ClosureObject, key string) string {
	colon := -1
	for i := 0; i < len(key); i++ {
		if key[i] == ':' {
			colon = i
			break
		}
		if key[i] < '0' || key[i] > '9' {
			return key
		}
	}
	if colon <= 0 {
		return key
	}
	classID, err := strconv.Atoi(key[:colon])
	if err != nil {
		return key
	}
	for env := vm.currentPrivateEnv(closure); env != nil; env = env.outer {
		if env.classID != classID {
			continue
		}
		if env.eval <= 1 {
			return key
		}
		if k, ok := env.keys[key]; ok {
			return k
		}
		if env.keys == nil {
			env.keys = make(map[string]string)
		}
		k := key + "@" + strconv.Itoa(env.eval)
		env.keys[key] = k
		return k
	}
	return key
}

// privateDisplayName drops a resolved key's evaluation suffix ("m@2" -> "m").
func privateDisplayName(name string) string {
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '@' {
			return name[:i]
		}
	}
	return name
}
