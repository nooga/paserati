package compiler

import (
	"fmt"
	"math"
	"math/big"
	"sort"
	"strings"

	"github.com/nooga/paserati/pkg/checker"
	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/lexer"
	"github.com/nooga/paserati/pkg/modules"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

// Define a placeholder register value for 'undefined' case
// Also used temporarily for recursive function definition
const nilRegister Register = 255 // Or another value guaranteed not to be used

// UpvalueCaptureType indicates where to capture a variable from when creating a closure.
type UpvalueCaptureType byte

const (
	// CaptureFromUpvalue captures from the enclosing closure's upvalues array.
	// The index is the upvalue index in the parent closure (8-bit, 0-255).
	CaptureFromUpvalue UpvalueCaptureType = 0

	// CaptureFromRegister captures from a register in the current call frame.
	// The index is the register number.
	CaptureFromRegister UpvalueCaptureType = 1

	// CaptureFromSpill captures from a spill slot in the current call frame.
	// The index is the spill slot index (8-bit, 0-255). The VM creates a closed upvalue directly.
	CaptureFromSpill UpvalueCaptureType = 2

	// CaptureFromSpill16 captures from a spill slot with a 16-bit index (256-65535).
	// Used when spill slot index exceeds 255. Requires 2 bytes for the index.
	CaptureFromSpill16 UpvalueCaptureType = 3

	// CaptureFromUpvalue16 captures from the enclosing closure's upvalues array with a 16-bit index (256-65535).
	// Used when upvalue index exceeds 255. Requires 2 bytes for the index.
	CaptureFromUpvalue16 UpvalueCaptureType = 4
)

// getExportSpecName extracts the name string from an export specifier's Local or Exported field
// which can be either an Identifier or StringLiteral (ES2022 string export names)
func getExportSpecName(expr parser.Expression) string {
	if expr == nil {
		return ""
	}
	if ident, ok := expr.(*parser.Identifier); ok {
		return ident.Value
	}
	if strLit, ok := expr.(*parser.StringLiteral); ok {
		return strLit.Value
	}
	return ""
}

// collectDestructuringNames extracts variable names from object destructuring properties
func collectDestructuringNames(props []*parser.DestructuringProperty, rest *parser.DestructuringElement, seen map[string]bool, names *[]string) {
	for _, prop := range props {
		if prop == nil || prop.Target == nil {
			continue
		}
		collectPatternNames(prop.Target, seen, names)
	}
	if rest != nil && rest.Target != nil {
		collectPatternNames(rest.Target, seen, names)
	}
}

// collectArrayDestructuringNames extracts variable names from array destructuring elements
func collectArrayDestructuringNames(elements []*parser.DestructuringElement, seen map[string]bool, names *[]string) {
	for _, elem := range elements {
		if elem == nil || elem.Target == nil {
			continue
		}
		collectPatternNames(elem.Target, seen, names)
	}
}

// collectPatternNames recursively extracts variable names from a destructuring pattern target
func collectPatternNames(target parser.Expression, seen map[string]bool, names *[]string) {
	if target == nil {
		return
	}
	switch t := target.(type) {
	case *parser.Identifier:
		if !seen[t.Value] {
			*names = append(*names, t.Value)
			seen[t.Value] = true
		}
	case *parser.ObjectLiteral:
		// Nested object pattern: { a: { b, c } }
		for _, prop := range t.Properties {
			if prop == nil {
				continue
			}
			if prop.Value != nil {
				// Key: Value pattern - the target is in Value
				collectPatternNames(prop.Value, seen, names)
			} else if prop.Key != nil {
				// Shorthand pattern { a } - Key is both the source and target
				collectPatternNames(prop.Key, seen, names)
			}
		}
	case *parser.ArrayLiteral:
		// Nested array pattern: [a, [b, c]]
		for _, elem := range t.Elements {
			collectPatternNames(elem, seen, names)
		}
	}
}

// extractDestructuringVarNames extracts all variable names from an ObjectDestructuringDeclaration
// This is a wrapper that creates a new seen map for use in block predefine pass
func extractDestructuringVarNames(decl *parser.ObjectDestructuringDeclaration) []string {
	var names []string
	seen := make(map[string]bool)
	collectDestructuringNames(decl.Properties, decl.RestProperty, seen, &names)
	return names
}

// extractArrayDestructuringVarNames extracts all variable names from an ArrayDestructuringDeclaration
// This is a wrapper that creates a new seen map for use in block predefine pass
func extractArrayDestructuringVarNames(decl *parser.ArrayDestructuringDeclaration) []string {
	var names []string
	seen := make(map[string]bool)
	collectArrayDestructuringNames(decl.Elements, seen, &names)
	return names
}

// collectVarDeclarations recursively collects all var declaration names from statements.
// This is used for var hoisting - var declarations are hoisted to the top of their
// function/script scope and initialized to undefined.
func collectVarDeclarations(stmts []parser.Statement) []string {
	var names []string
	seen := make(map[string]bool)

	var collect func(stmt parser.Statement)
	collect = func(stmt parser.Statement) {
		if stmt == nil {
			return
		}
		switch s := stmt.(type) {
		case *parser.VarStatement:
			for _, decl := range s.Declarations {
				if decl.Name != nil && !seen[decl.Name.Value] {
					names = append(names, decl.Name.Value)
					seen[decl.Name.Value] = true
				}
			}
		case *parser.ObjectDestructuringDeclaration:
			// Only hoist if it's a 'var' declaration (Token.Literal == "var")
			if s.Token.Literal == "var" {
				collectDestructuringNames(s.Properties, s.RestProperty, seen, &names)
			}
		case *parser.ArrayDestructuringDeclaration:
			// Only hoist if it's a 'var' declaration (Token.Literal == "var")
			if s.Token.Literal == "var" {
				collectArrayDestructuringNames(s.Elements, seen, &names)
			}
		case *parser.BlockStatement:
			if s != nil && s.Statements != nil {
				for _, inner := range s.Statements {
					collect(inner)
				}
			}
		case *parser.IfStatement:
			if s.Consequence != nil {
				collect(s.Consequence)
			}
			if s.Alternative != nil {
				collect(s.Alternative)
			}
		case *parser.WhileStatement:
			if s.Body != nil {
				collect(s.Body)
			}
		case *parser.DoWhileStatement:
			if s.Body != nil {
				collect(s.Body)
			}
		case *parser.ForStatement:
			if s.Initializer != nil {
				collect(s.Initializer)
			}
			if s.Body != nil {
				collect(s.Body)
			}
		case *parser.ForInStatement:
			// For-in initializer can be a var declaration
			if s.Variable != nil {
				if varStmt, ok := s.Variable.(*parser.VarStatement); ok {
					collect(varStmt)
				}
			}
			if s.Body != nil {
				collect(s.Body)
			}
		case *parser.ForOfStatement:
			// For-of initializer can be a var declaration
			if s.Variable != nil {
				if varStmt, ok := s.Variable.(*parser.VarStatement); ok {
					collect(varStmt)
				}
			}
			if s.Body != nil {
				collect(s.Body)
			}
		case *parser.SwitchStatement:
			if s.Cases != nil {
				for _, caseClause := range s.Cases {
					if caseClause != nil && caseClause.Body != nil && caseClause.Body.Statements != nil {
						for _, inner := range caseClause.Body.Statements {
							collect(inner)
						}
					}
				}
			}
		case *parser.TryStatement:
			if s.Body != nil {
				collect(s.Body)
			}
			if s.CatchClause != nil && s.CatchClause.Body != nil {
				collect(s.CatchClause.Body)
			}
			if s.FinallyBlock != nil {
				collect(s.FinallyBlock)
			}
		case *parser.WithStatement:
			if s.Body != nil {
				collect(s.Body)
			}
		case *parser.LabeledStatement:
			if s.Statement != nil {
				collect(s.Statement)
			}
		}
	}

	for _, stmt := range stmts {
		collect(stmt)
	}
	return names
}

// collectLetConstDeclarations collects top-level let and const declaration names from statements.
// Unlike var, let/const are block-scoped, so we only collect from the immediate statement list.
// This is used for TDZ (Temporal Dead Zone) - let/const must be initialized with the
// Uninitialized marker at script start, and only get their actual value when the declaration
// line is executed. Accessing them before declaration triggers a ReferenceError.
func collectLetConstDeclarations(stmts []parser.Statement) []string {
	var names []string
	seen := make(map[string]bool)

	for _, stmt := range stmts {
		if stmt == nil {
			continue
		}
		switch s := stmt.(type) {
		case *parser.LetStatement:
			if s.Name != nil && !seen[s.Name.Value] {
				names = append(names, s.Name.Value)
				seen[s.Name.Value] = true
			}
		case *parser.ConstStatement:
			if s.Name != nil && !seen[s.Name.Value] {
				names = append(names, s.Name.Value)
				seen[s.Name.Value] = true
			}
		case *parser.ObjectDestructuringDeclaration:
			// let/const destructuring
			if s.Token.Literal == "let" || s.Token.Literal == "const" {
				collectDestructuringNames(s.Properties, s.RestProperty, seen, &names)
			}
		case *parser.ArrayDestructuringDeclaration:
			// let/const destructuring
			if s.Token.Literal == "let" || s.Token.Literal == "const" {
				collectArrayDestructuringNames(s.Elements, seen, &names)
			}
		}
	}
	return names
}

// --- New: Loop Context for Break/Continue ---
type LoopContext struct {
	// Optional label for this loop/statement
	Label string
	// Start of the loop condition check (target for continue in while)
	LoopStartPos int
	// Start of the update expression (target for continue in for)
	// Set to LoopStartPos for while loops
	ContinueTargetPos int
	// List of bytecode positions where OpJump placeholders for break statements start.
	// These need to be patched later to the loop's end address.
	BreakPlaceholderPosList []int
	// List of bytecode positions where OpJump placeholders for continue statements start.
	ContinuePlaceholderPosList []int
	// Iterator cleanup information for for...of loops using iterator protocol
	IteratorCleanup *IteratorCleanupInfo
	// Completion value register for this loop (for break/continue to set to undefined per UpdateEmpty)
	CompletionReg Register
}

// IteratorCleanupInfo tracks iterator objects that need cleanup on early exit
type IteratorCleanupInfo struct {
	// Register containing the iterator object (has .return() method)
	IteratorReg Register
	// Whether this loop uses the iterator protocol (vs fast array path)
	UsesIteratorProtocol bool
}

// FinallyContext tracks try-finally blocks for proper control flow handling
type FinallyContext struct {
	// PC of the finally block (where to jump when break/continue/return occurs)
	FinallyPC int
	// List of placeholder jump positions that need patching to point to finally
	// These are jumps FROM break/continue/return statements TO the finally block
	JumpToFinallyPlaceholders []int
	// Loop stack depth when this finally context was created
	// Used to determine if a break/continue targets a loop outside the try-finally
	LoopStackDepthAtCreation int
}

const debugCompiler = false // Set to true to trace compiler output
const debugCompilerStats = false
const debugCompiledCode = false // Enable disassembly output
const debugPrint = false        // Enable debug output

// Feature flag: Enable Tail Call Optimization
const enableTCO = true // Set to false to disable TCO for baseline testing

func debugPrintf(format string, args ...interface{}) {
	if debugCompiler {
		fmt.Printf(format, args...)
	}
}

type CompilerStats struct {
	BytesGenerated int
}

// Compiler transforms an AST into bytecode.
type Compiler struct {
	chunk              *vm.Chunk
	regAlloc           *RegisterAllocator
	currentSymbolTable *SymbolTable
	enclosing          *Compiler
	freeSymbols        []*Symbol
	errors             []errors.PaseratiError
	loopContextStack   []*LoopContext
	compilingFuncName  string
	typeChecker        *checker.Checker // Holds the checker instance
	stats              *CompilerStats

	// --- NEW: Global Variable Indexing for Performance ---
	// Maps global variable names to their assigned indices (only for top-level compiler)
	globalIndices map[string]int
	// Count of global variables assigned so far (only for top-level compiler)
	globalCount int
	// Unified heap allocator for coordinating global indices across modules
	heapAlloc *HeapAlloc
	// line tracking
	line int
	// Anonymous class counter for generating unique names
	anonymousClassCounter int

	// --- NEW: Constant Register Cache for Performance ---
	// Maps constant index to register that currently holds it
	constantCache map[uint16]Register

	// --- Phase 4a: Finally Context Tracking ---
	inFinallyBlock  bool // Track if we're compiling inside finally block
	tryFinallyDepth int  // Number of enclosing try-with-finally blocks

	// --- Block Scope Tracking ---
	isCompilingFunctionBody bool              // Track if we're compiling the function body BlockStatement itself
	tryDepth                int               // Number of enclosing try blocks (any kind: try-catch, try-finally, try-catch-finally)
	finallyContextStack     []*FinallyContext // Stack of active finally contexts
	withBlockDepth          int               // Total with blocks (inherited, for unresolved var lookup)
	currentFuncWithDepth    int               // With blocks in current function only (NOT inherited, for local var lookup)

	// --- Strict Mode Inheritance ---
	inheritedStrictMode bool // Inherited strict mode from eval context

	// --- Upvalue Optimization ---
	hasLocalCaptures bool // True if any nested closure captures locals from this function

	// --- Phase 5: Module Bindings ---
	moduleBindings *ModuleBindings      // Module-aware binding resolver
	moduleLoader   modules.ModuleLoader // Reference to module loader

	// --- Module Import Deduplication ---
	processedModules map[string]bool // Track modules already processed to avoid duplicate OpEvalModule

	// --- Type Error Handling ---
	ignoreTypeErrors bool // When true, compilation continues despite type errors
	skipTypeCheck    bool // When true, type checker is not run at all (for pure JS mode)

	// --- NEW: Class Context for super() support ---
	compilingSuperClassName string // Name of parent class when compiling derived class constructor

	// --- Tail Call Optimization ---
	inTailPosition bool // True when compiling tail-positioned expression

	// --- Function Context Tracking ---
	isAsync              bool // True when compiling async function
	isGenerator          bool // True when compiling generator function
	isArrowFunction      bool // True when compiling arrow function (no own arguments binding)
	isFunctionBody       bool // True when compiling a function body (has implicit 'arguments' binding, not module-level)
	isMethodCompilation      bool // True when compiling a method that will have [[HomeObject]] (class/object method)
	isClassFieldInitializer  bool // True when compiling a class field initializer (eval shouldn't access 'arguments')

	// --- Direct Eval Tracking ---
	hasDirectEval            bool // True when function contains direct eval call (needs scope descriptor)
	inDefaultParamScope      bool // True when compiling a default parameter expression
	hasEvalInDefaultParam    bool // True when direct eval was found in a default parameter expression
	hasEvalInFieldInitializer bool // True when direct eval was found in a class field initializer
	allLocalNames            map[Register]string // Tracks all local variable allocations for ScopeDescriptor

	// --- Parameter TDZ Tracking ---
	currentDefaultParamIndex int      // Index of parameter whose default we're compiling (-1 if not in default scope)
	parameterList            []string // Ordered list of parameter names for TDZ checking

	// --- Caller Scope for Direct Eval Compilation ---
	callerScopeDesc *vm.ScopeDescriptor // Scope descriptor from caller (for direct eval compilation)

	// --- Indirect Eval Mode ---
	isIndirectEval      bool // True when compiling indirect eval code (let/const should be local, not global)
	newTargetAvailable  bool // True if new.target is available in current context (false for indirect eval at global scope)

	// --- Script Mode for Function Constructor ---
	forceScriptMode bool // True when compiling code from Function() constructor (import.meta not allowed)

	// --- Parameter Names Tracking ---
	parameterNames map[string]bool // Set of parameter names for current function (for var hoisting)

	// --- Register Spilling Support ---
	nextSpillSlot uint16 // Next available spill slot index (0-65534)

	// --- Scope Boundary Tracking ---
	// scopeBoundary marks the first symbol table that belongs to an enclosing compiler.
	// When walking the Outer chain, we stop at this table to avoid crossing compiler boundaries.
	// For top-level compilers, this is nil.
	scopeBoundary *SymbolTable

	// --- Private Field Brand Tracking ---
	// Each class gets a unique brand ID to distinguish private fields with the same name
	// across different classes (e.g., Parent.#field vs Child.#field)
	privateBrandCounter      int                // Global counter for brand IDs (at top-level compiler)
	currentPrivateBrand      int                // Current class brand ID (0 if not in a class)
	currentPrivateBrandInfo  PrivateBrandInfo   // Current class brand info including declared fields
	privateBrandStack        []PrivateBrandInfo // Stack for nested classes (with declared field tracking)

	// --- Computed Field Key Pre-evaluation ---
	// Maps property index to synthetic variable name for pre-computed field keys
	// Per ECMAScript, computed property keys must be evaluated at class definition time, not instantiation
	computedFieldKeyVars map[int]string
}

// NewCompiler creates a new *top-level* Compiler.
func NewCompiler() *Compiler {
	return &Compiler{
		chunk:               vm.NewChunk(),
		regAlloc:            NewRegisterAllocator(),
		currentSymbolTable:  NewSymbolTable(),
		enclosing:           nil,
		freeSymbols:         []*Symbol{},
		errors:              []errors.PaseratiError{},
		loopContextStack:    make([]*LoopContext, 0),
		compilingFuncName:   "<script>",
		typeChecker:         nil, // Initialized to nil, can be set externally
		stats:               &CompilerStats{},
		globalIndices:       make(map[string]int),
		globalCount:         0,
		line:                -1,
		constantCache:       make(map[uint16]Register),
		processedModules:    make(map[string]bool),
		finallyContextStack: make([]*FinallyContext, 0),
		allLocalNames:       make(map[Register]string),
	}
}

// SetChecker allows injecting an external checker instance.
// This is used by the driver for REPL sessions.
func (c *Compiler) SetChecker(checker *checker.Checker) {
	c.typeChecker = checker
}

// SetIgnoreTypeErrors sets whether to continue compilation despite type errors
func (c *Compiler) SetIgnoreTypeErrors(ignore bool) {
	c.ignoreTypeErrors = ignore
}

// SetSkipTypeCheck sets whether to completely skip type checking
// When true, the type checker is not run at all (for pure JS mode with no type annotations)
func (c *Compiler) SetSkipTypeCheck(skip bool) {
	c.skipTypeCheck = skip
}

// SetHeapAlloc sets the heap allocator for coordinating global indices
func (c *Compiler) SetHeapAlloc(heapAlloc *HeapAlloc) {
	c.heapAlloc = heapAlloc
}

// AllocSpillSlot allocates a new spill slot for storing a variable when registers are exhausted.
// Returns the spill slot index (0-65534). Panics if spill slots are exhausted.
func (c *Compiler) AllocSpillSlot() uint16 {
	if c.nextSpillSlot == 65535 {
		panic("Compiler Error: Ran out of spill slots!")
	}
	slot := c.nextSpillSlot
	c.nextSpillSlot++
	return slot
}

// GetHeapAlloc returns the compiler's heap allocator
func (c *Compiler) GetHeapAlloc() *HeapAlloc {
	return c.heapAlloc
}

// isDefinedInEnclosingCompiler checks if a symbol table belongs to ANY enclosing compiler's scope chain.
// This helps distinguish between:
// - Variables from outer block scopes in the SAME function (return false) -> use direct register
// - Variables from outer FUNCTION scopes (return true) -> use OpLoadFree (upvalue)
// Note: This recursively checks ALL enclosing compilers, not just the immediate parent.
// This is important for deeply nested closures (e.g., IIFE -> function -> arrow).
func (c *Compiler) isDefinedInEnclosingCompiler(definingTable *SymbolTable) bool {
	if c.enclosing == nil {
		debugPrintf("// [isDefinedInEnclosingCompiler] c.enclosing is nil, returning false\n")
		return false
	}

	debugPrintf("// [isDefinedInEnclosingCompiler] Checking definingTable=%p against c.enclosing.currentSymbolTable=%p (compilingFuncName=%s)\n",
		definingTable, c.enclosing.currentSymbolTable, c.enclosing.compilingFuncName)

	// Walk the immediate enclosing compiler's symbol table chain
	for table := c.enclosing.currentSymbolTable; table != nil; table = table.Outer {
		debugPrintf("// [isDefinedInEnclosingCompiler]   Walking table=%p\n", table)
		if table == definingTable {
			debugPrintf("// [isDefinedInEnclosingCompiler]   MATCH FOUND! Returning true\n")
			return true
		}
	}

	debugPrintf("// [isDefinedInEnclosingCompiler]   No match in immediate enclosing, recursing...\n")
	// Recursively check grandparent and beyond
	return c.enclosing.isDefinedInEnclosingCompiler(definingTable)
}

// isInCurrentScopeChain checks if a symbol table is part of this compiler's scope chain.
// This walks the Outer chain from currentSymbolTable but STOPS at scopeBoundary
// (which marks where the parent compiler's scope begins).
// Used for closure emission to correctly identify variables that are local to the current function
// vs variables from outer functions that need upvalue capture.
func (c *Compiler) isInCurrentScopeChain(table *SymbolTable) bool {
	for t := c.currentSymbolTable; t != nil && t != c.scopeBoundary; t = t.Outer {
		if t == table {
			return true
		}
	}
	return false
}

// EnableModuleMode enables module-aware compilation with binding resolution
// Parallels the checker's EnableModuleMode method
func (c *Compiler) EnableModuleMode(modulePath string, loader modules.ModuleLoader) {
	c.moduleBindings = NewModuleBindings(modulePath, loader)
	c.moduleLoader = loader

	// If we have a type checker, synchronize import information from it
	if c.typeChecker != nil && c.typeChecker.IsModuleMode() {
		c.syncImportsFromTypeChecker()
	}
}

// IsModuleMode returns true if the compiler is in module mode
func (c *Compiler) IsModuleMode() bool {
	return c.moduleBindings != nil
}

// SetStrictMode sets the initial strict mode for the compiler
// This is used by eval() to compile code in strict mode when called from strict context
func (c *Compiler) SetStrictMode(strict bool) {
	c.inheritedStrictMode = strict
}

// SetForceScriptMode forces script mode for compilation
// When true, import.meta is not allowed even if the parent is in module mode
// This is used by Function() constructor to compile code as Script (not Module)
func (c *Compiler) SetForceScriptMode(force bool) {
	c.forceScriptMode = force
}

// syncImportsFromTypeChecker synchronizes import information from the type checker
// This ensures that imports processed during type checking are available during compilation
func (c *Compiler) syncImportsFromTypeChecker() {
	debugPrintf("// [Compiler] syncImportsFromTypeChecker called. typeChecker=%v, isModuleMode=%v, moduleBindings=%v\n",
		c.typeChecker != nil,
		c.typeChecker != nil && c.typeChecker.IsModuleMode(),
		c.moduleBindings != nil)

	if c.typeChecker == nil || !c.typeChecker.IsModuleMode() || c.moduleBindings == nil {
		debugPrintf("// [Compiler] syncImportsFromTypeChecker early return\n")
		return
	}

	// Get import bindings from the type checker
	importBindings := c.typeChecker.GetImportBindings()
	if importBindings == nil {
		return
	}

	debugPrintf("// [Compiler] Syncing %d import bindings from type checker\n", len(importBindings))

	// Convert type checker's ImportBinding to compiler's ImportReference
	for localName, binding := range importBindings {
		// Convert ImportBindingType to ImportReferenceType
		var importType ImportReferenceType
		switch binding.ImportType {
		case 0: // ImportDefault from checker
			importType = ImportDefaultRef
		case 1: // ImportNamed from checker
			importType = ImportNamedRef
		case 2: // ImportNamespace from checker
			importType = ImportNamespaceRef
		default:
			importType = ImportNamedRef // Default fallback
		}

		// Get or assign global index for this import
		globalIndex := c.GetOrAssignGlobalIndex(localName)

		// Add to module bindings
		c.moduleBindings.DefineImport(localName, binding.SourceModule, binding.SourceName, importType, int(globalIndex))

		debugPrintf("// [Compiler] Synced import: %s from %s (global index: %d)\n",
			localName, binding.SourceModule, globalIndex)
	}
}

// GetModuleExports returns all exported values from the current module
func (c *Compiler) GetModuleExports() map[string]vm.Value {
	if c.moduleBindings == nil {
		return make(map[string]vm.Value)
	}
	return c.moduleBindings.GetAllExports()
}

// newFunctionCompiler creates a compiler instance specifically for a function body.
func newFunctionCompiler(enclosingCompiler *Compiler) *Compiler {
	// Pass the checker and module bindings down to nested compilers
	chunk := vm.NewChunk()
	// Inherit strict mode from enclosing compiler
	chunk.IsStrict = enclosingCompiler.chunk.IsStrict
	return &Compiler{
		chunk:                    chunk,
		regAlloc:                 NewRegisterAllocator(),
		currentSymbolTable:       NewEnclosedSymbolTable(enclosingCompiler.currentSymbolTable),
		enclosing:                enclosingCompiler,
		freeSymbols:              []*Symbol{},
		errors:                   []errors.PaseratiError{},
		loopContextStack:         make([]*LoopContext, 0),
		compilingFuncName:        "",
		typeChecker:              enclosingCompiler.typeChecker, // Inherit checker from enclosing
		stats:                    enclosingCompiler.stats,
		constantCache:            make(map[uint16]Register),                 // Each function has its own constant cache
		moduleBindings:           enclosingCompiler.moduleBindings,          // Inherit module bindings
		moduleLoader:             enclosingCompiler.moduleLoader,            // Inherit module loader
		compilingSuperClassName:  enclosingCompiler.compilingSuperClassName, // Inherit super class context
		finallyContextStack:      make([]*FinallyContext, 0),                // Each function has its own finally context stack
		withBlockDepth:           enclosingCompiler.withBlockDepth,          // Inherit for unresolved var lookups in closure's scope chain
		currentFuncWithDepth:     0,                                         // NOT inherited - function's own locals shadow with-object
		parameterNames:           make(map[string]bool),                     // Track parameter names for var hoisting
		currentDefaultParamIndex: -1,                                        // Not in default param scope initially
		parameterList:            nil,                                       // Will be set when compiling function parameters
		scopeBoundary:            enclosingCompiler.currentSymbolTable,      // Mark where parent's scope starts
		allLocalNames:            make(map[Register]string),                 // Track all local names for ScopeDescriptor
		isClassFieldInitializer: enclosingCompiler.isClassFieldInitializer, // Inherit field initializer context for nested functions
		// Inherit private field brand context - methods need to use the class's brand ID
		// and also need the brand stack to look up fields from enclosing classes
		currentPrivateBrand:     enclosingCompiler.currentPrivateBrand,
		currentPrivateBrandInfo: enclosingCompiler.currentPrivateBrandInfo,
		privateBrandStack:       enclosingCompiler.privateBrandStack, // Inherited so nested can access outer class fields
		// Inherit script mode from Function() constructor - import.meta not allowed in nested functions
		forceScriptMode:         enclosingCompiler.forceScriptMode,
	}
}

// Compile traverses the AST, performs type checking using its assigned checker,
// and generates bytecode.
// Returns the generated chunk and any errors encountered (including type errors).
func (c *Compiler) Compile(node parser.Node) (*vm.Chunk, []errors.PaseratiError) {

	// --- Type Checking Step ---
	program, ok := node.(*parser.Program)
	if !ok {
		// Compiler currently expects the root node to be a Program.
		// If not, it cannot type check. Return an immediate internal compiler error.
		// We create a placeholder token for the position.
		// TODO: Find a better way to get position info if input isn't a Program.
		placeholderToken := lexer.Token{Type: lexer.ILLEGAL, Literal: "", Line: 1, Column: 1, StartPos: 0, EndPos: 0}
		compileErr := &errors.CompileError{
			Position: errors.Position{
				Line:     placeholderToken.Line,
				Column:   placeholderToken.Column,
				StartPos: placeholderToken.StartPos,
				EndPos:   placeholderToken.EndPos,
			},
			Msg: "compiler error: Compile input must be *parser.Program for type checking",
		}
		// Append to errors list as well
		c.errors = append(c.errors, compileErr)
		return nil, c.errors
	}

	// Use the assigned checker. If none was assigned (e.g., non-REPL), create one.
	if c.typeChecker == nil {
		c.typeChecker = checker.NewChecker()
	}

	// Skip type checking entirely if skipTypeCheck is set (pure JS mode)
	if !c.skipTypeCheck {
		// For direct eval compilation with super binding, tell the checker that super is allowed
		if c.callerScopeDesc != nil && c.callerScopeDesc.HasSuperBinding {
			c.typeChecker.SetAllowSuperInEval(true)
			defer c.typeChecker.SetAllowSuperInEval(false) // Reset after type checking
		}

		// Check if this program has already been type-checked by comparing the AST
		// In module loading flow, the checker is already used to check the program
		if c.typeChecker.GetProgram() != program {
			// Program hasn't been checked yet, perform type checking
			// The checker modifies its own environment state.
			typeErrors := c.typeChecker.Check(program)
			if len(typeErrors) > 0 && !c.ignoreTypeErrors {
				// Found type errors and not ignoring them. Return them immediately.
				// Type errors are already []errors.PaseratiError from the checker.
				return nil, typeErrors
			}
		} else {
			// Program was already type-checked by this checker, skip redundant check
			debugPrintf("// [Compiler] Skipping type check - program already checked by this checker\n")
		}
	} else {
		debugPrintf("// [Compiler] Skipping type check entirely (skipTypeCheck=true)\n")
	}
	// No need to re-assign c.typeChecker here, it was already set or created.
	// --- End Type Checking Step ---

	// --- Bytecode Compilation Step ---
	c.chunk = vm.NewChunk()
	c.regAlloc = NewRegisterAllocator()
	c.currentSymbolTable = NewSymbolTable()
	c.allLocalNames = make(map[Register]string) // Reset local name tracking

	// Re-populate symbol table with caller locals for direct eval
	// This must be done after resetting the symbol table but before compilation
	// We also need to track which registers need to be populated from caller scope
	var callerLocalRegs []struct {
		reg        Register
		callerIdx  int
	}
	if c.callerScopeDesc != nil {
		for i, name := range c.callerScopeDesc.LocalNames {
			if name != "" {
				reg := c.regAlloc.Alloc()
				c.currentSymbolTable.DefineCallerLocal(name, i)
				// Update the symbol to have a register as well (for upvalue capture)
				if sym, found := c.currentSymbolTable.store[name]; found {
					sym.Register = reg
					c.currentSymbolTable.store[name] = sym
					debugPrintf("// [Compile] Re-populated caller local '%s' at index %d with register R%d\n", name, i, reg)
					callerLocalRegs = append(callerLocalRegs, struct {
						reg        Register
						callerIdx  int
					}{reg, i})
				}
			}
		}
	}

	// If the caller is inside a class field initializer, propagate that context
	// This is needed for the "Additional Early Error Rules for Eval Inside Initializer"
	// which forbid 'arguments' in eval code and any nested functions within it
	if c.callerScopeDesc != nil && c.callerScopeDesc.InClassFieldInitializer {
		c.isClassFieldInitializer = true
	}

	// Restore private brand context from caller scope for direct eval
	// This allows eval code to access private fields from enclosing classes
	if c.callerScopeDesc != nil {
		c.restoreBrandContextFromVM(c.callerScopeDesc)
	}

	// Reset per-compilation state to avoid leaking state between eval calls
	c.loopContextStack = nil
	c.finallyContextStack = nil
	c.errors = nil
	c.inFinallyBlock = false
	c.tryFinallyDepth = 0
	c.tryDepth = 0
	c.constantCache = nil

	// --- Determine strict mode ---
	// TypeScript mode (type checking enabled): default to strict mode
	// JavaScript mode (type checking disabled or skipped): respect "use strict" directive or inherited strict mode
	if !c.ignoreTypeErrors && !c.skipTypeCheck {
		// TypeScript mode - always strict
		c.chunk.IsStrict = true
		debugPrintf("[Compile] TypeScript mode - enabling strict mode by default\n")
	} else {
		// JavaScript mode - check for inherited strict mode (from eval context) first
		if c.inheritedStrictMode {
			c.chunk.IsStrict = true
		} else {
			// Check for "use strict" directive in the directive prologue
			// Per ECMAScript spec, iterate through consecutive string literal ExpressionStatements
			for _, stmt := range program.Statements {
				exprStmt, ok := stmt.(*parser.ExpressionStatement)
				if !ok {
					// Not an ExpressionStatement - directive prologue ends
					break
				}
				strLit, ok := exprStmt.Expression.(*parser.StringLiteral)
				if !ok {
					// ExpressionStatement but not a string literal - directive prologue ends
					break
				}
				// This is a directive - check if it's "use strict"
				if strLit.Value == "use strict" {
					c.chunk.IsStrict = true
					debugPrintf("[Compile] Detected 'use strict' directive - enabling strict mode\n")
					break
				}
				// Otherwise it's another directive, continue checking
			}
		}
	}

	// --- Global Symbol Table Initialization (if needed) ---
	// c.defineBuiltinGlobals() // TODO: Define built-ins if any

	// --- Emit OpGetCallerLocal for direct eval caller locals ---
	// This must happen at the start of eval code execution so nested functions
	// can capture the values as upvalues correctly
	for _, callerLocal := range callerLocalRegs {
		c.emitOpCode(vm.OpGetCallerLocal, 0)
		c.emitByte(byte(callerLocal.reg))
		c.emitByte(byte(callerLocal.callerIdx))
		debugPrintf("// [Compile] Emitted OpGetCallerLocal R%d from caller index %d\n", callerLocal.reg, callerLocal.callerIdx)
	}

	// --- Process Runtime Import Declarations FIRST (before hoisted functions) ---
	// This ensures that runtime imported names are available when compiling hoisted function bodies
	// Type-only imports are handled by the type checker and synced via syncImportsFromTypeChecker
	if c.IsModuleMode() {
		debugPrintf("[Compile] Processing runtime import declarations before hoisted functions...\n")
		for _, stmt := range program.Statements {
			if importDecl, ok := stmt.(*parser.ImportDeclaration); ok {
				debugPrintf("[Compile] Pre-processing import from: %s (IsTypeOnly: %v)\n", importDecl.Source.Value, importDecl.IsTypeOnly)

				// Only process runtime imports here, skip type-only imports
				// Type-only imports are already processed by the type checker
				if !importDecl.IsTypeOnly {
					_, err := c.compileNode(importDecl, c.regAlloc.Alloc())
					if err != nil {
						debugPrintf("[Compile] ERROR processing runtime import: %v\n", err)
						// Continue with other imports even if one fails
					}
				} else {
					debugPrintf("[Compile] Skipping type-only import from: %s\n", importDecl.Source.Value)
				}
			}
		}
		debugPrintf("[Compile] Finished pre-processing runtime imports\n")
	}

	// --- Pre-register global declaration names BEFORE compiling hoisted functions ---
	// This ensures that when hoisted functions try to assign to global vars,
	// c.GlobalExists() returns true and strict mode doesn't throw ReferenceError.
	// The actual initialization (setting to undefined) happens later.
	// We need to pre-register var, let, and const declarations because they're all
	// globals at module/script scope.
	if c.enclosing == nil {
		// Pre-register var declarations
		varNames := collectVarDeclarations(program.Statements)
		for _, name := range varNames {
			c.GetOrAssignGlobalIndex(name)
		}
		// Pre-register let/const declarations (except for eval which has different scoping)
		if !c.isIndirectEval && c.callerScopeDesc == nil {
			letConstNames := collectLetConstDeclarations(program.Statements)
			for _, name := range letConstNames {
				c.GetOrAssignGlobalIndex(name)
			}
			debugPrintf("[Compile] Pre-registered %d var and %d let/const names for strict mode checks\n", len(varNames), len(letConstNames))
		} else {
			debugPrintf("[Compile] Pre-registered %d var names for strict mode checks\n", len(varNames))
		}
	}

	// --- Compile Hoisted Global Functions AFTER imports are processed ---
	if program.HoistedDeclarations != nil {
		debugPrintf("[Compile] Processing %d hoisted global declarations...\n", len(program.HoistedDeclarations))
		// Sort hoisted function names for deterministic compilation order
		// Go maps have non-deterministic iteration order, which can cause
		// different register/spill slot assignments between runs
		hoistedGlobalNames := make([]string, 0, len(program.HoistedDeclarations))
		for name := range program.HoistedDeclarations {
			hoistedGlobalNames = append(hoistedGlobalNames, name)
		}
		sort.Strings(hoistedGlobalNames)
		for _, name := range hoistedGlobalNames {
			hoistedNode := program.HoistedDeclarations[name]
			funcLit, ok := hoistedNode.(*parser.FunctionLiteral)
			if !ok {
				// Should not happen if checker/parser worked correctly
				debugPrintf("[Compile Hoisting] ERROR: Hoisted node for '%s' is not FunctionLiteral (%T)\n", name, hoistedNode)
				continue
			}

			// 1. Define the function name temporarily to allow self-recursion
			c.currentSymbolTable.Define(name, nilRegister)

			// 2. Compile the function literal (creates chunk & function object)
			// This will now properly detect self-recursion and include it in freeSymbols
			funcConstIndex, freeSymbols, err := c.compileFunctionLiteral(funcLit, name)
			if err != nil {
				// Error during function compilation, already added to c.errors by sub-compiler
				debugPrintf("[Compile Hoisting] ERROR compiling hoisted func '%s': %v\n", name, err)
				continue // Skip defining this function
			}

			// 3. Create the Closure with the actual freeSymbols (not nil)
			closureReg := c.emitClosure(c.regAlloc.Alloc(), funcConstIndex, funcLit, freeSymbols) // Use actual freeSymbols

			// 4. Get global index and define as global symbol
			globalIdx := c.GetOrAssignGlobalIndex(name)

			// Update the symbol table entry to mark it as global
			c.currentSymbolTable.DefineGlobal(name, globalIdx)

			// 5. Emit OpSetGlobal to store the function in the VM's globals array
			c.emitSetGlobal(globalIdx, closureReg, funcLit.Token.Line)

			// 6. Mark as non-configurable (DontDelete) only for true top-level function declarations,
			// not for eval-created bindings which should be configurable
			if !c.isIndirectEval && c.callerScopeDesc == nil {
				c.MarkVarGlobal(globalIdx)
			}

			// 7. Free the temporary register now that the closure is stored in globals
			c.regAlloc.Free(closureReg)

			debugPrintf("[Compile Hoisting] Defined global func '%s' with %d upvalues in R%d, stored at global index %d\n", name, len(freeSymbols), closureReg, globalIdx)

		}
	}
	// --- END Hoisted Global Function Processing ---

	// --- Hoist var declarations at top-level (script scope) ---
	// Var declarations are hoisted to the top of their function/script scope
	// and initialized to undefined. This must happen before any statements execute.
	if c.enclosing == nil {
		varNames := collectVarDeclarations(program.Statements)
		debugPrintf("[Compile] Hoisting %d top-level var declarations\n", len(varNames))
		for _, name := range varNames {
			// Skip if already defined (e.g., by a hoisted function with the same name)
			if _, _, found := c.currentSymbolTable.Resolve(name); found {
				debugPrintf("[Compile VarHoist] Skipping '%s' - already defined\n", name)
				continue
			}
			// Define as global with undefined value
			globalIdx := c.GetOrAssignGlobalIndex(name)
			c.currentSymbolTable.DefineGlobal(name, globalIdx)
			// Emit code to initialize to undefined
			tempReg := c.regAlloc.Alloc()
			c.emitLoadUndefined(tempReg, 0)
			c.emitSetGlobal(globalIdx, tempReg, 0)
			c.regAlloc.Free(tempReg)
			// Mark as non-configurable (DontDelete) only for true top-level var declarations,
			// not for eval-created bindings which should be configurable
			if !c.isIndirectEval && c.callerScopeDesc == nil {
				c.MarkVarGlobal(globalIdx)
			}
			debugPrintf("[Compile VarHoist] Hoisted var '%s' at global index %d\n", name, globalIdx)
		}
	}
	// --- END var hoisting ---

	// --- Hoist let/const declarations with TDZ (Temporal Dead Zone) marker ---
	// Let/const declarations are initialized with the Uninitialized marker at script start.
	// This ensures that accessing them before their declaration line throws a ReferenceError.
	// Skip TDZ hoisting for eval code - eval has different scoping rules.
	if c.enclosing == nil && !c.isIndirectEval && c.callerScopeDesc == nil {
		letConstNames := collectLetConstDeclarations(program.Statements)
		debugPrintf("[Compile] TDZ hoisting %d top-level let/const declarations\n", len(letConstNames))
		for _, name := range letConstNames {
			// Skip if already defined (e.g., by a hoisted function with the same name - error in strict mode but allowed in sloppy)
			if _, _, found := c.currentSymbolTable.Resolve(name); found {
				debugPrintf("[Compile TDZHoist] Skipping '%s' - already defined\n", name)
				continue
			}
			// Define as global with Uninitialized value (TDZ)
			globalIdx := c.GetOrAssignGlobalIndex(name)
			c.currentSymbolTable.DefineGlobal(name, globalIdx)
			// Emit code to initialize to Uninitialized (TDZ marker)
			tempReg := c.regAlloc.Alloc()
			c.emitLoadUninitialized(tempReg, 0)
			c.emitSetGlobal(globalIdx, tempReg, 0)
			c.regAlloc.Free(tempReg)
			debugPrintf("[Compile TDZHoist] TDZ hoisted let/const '%s' at global index %d\n", name, globalIdx)
		}
	}
	// --- END let/const TDZ hoisting ---

	resultReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(resultReg)
	// Use the already type-checked program node.
	resultReg, err := c.compileNode(program, resultReg)
	if err != nil {
		// Treat any non-nil error from code generation as a hard compile failure.
		// Ensure it is included in the errors list, then abort without emitting epilogue.
		if len(c.errors) == 0 {
			c.errors = append(c.errors, err)
		}
		return nil, c.errors
	}

	// Emit final return instruction if no *compilation* errors occurred
	// (Type errors were caught earlier and returned).
	if len(c.errors) == 0 {
		if c.enclosing == nil { // Top-level script
			// If resultReg is BadRegister, no statement produced a value, so return undefined
			if resultReg == BadRegister {
				c.emitOpCode(vm.OpReturnUndefined, 0)
			} else {
				c.emitReturn(resultReg, 0)
			}
		} else {
			// Inside a function, OpReturn or OpReturnUndefined should have been emitted.
			// Add one just in case of missing return paths (though type checker might catch this).
			c.emitOpCode(vm.OpReturnUndefined, 0)
		}
	}

	c.stats.BytesGenerated += len(c.chunk.Code)
	if debugCompilerStats && c.enclosing == nil {
		fmt.Printf("// Compiler bytes generated: %d\n", c.stats.BytesGenerated)
	}

	// <<< ADDED: Log final register allocator state for debugging register leaks >>>
	if debugCompiler && c.enclosing == nil {
		fmt.Printf("// [FINAL REGALLOC STATE] NextReg: R%d, MaxReg: R%d, FreeRegs: %v, Pinned: %d\n",
			c.regAlloc.nextReg, c.regAlloc.maxReg+1, c.regAlloc.freeRegs, len(c.regAlloc.pinnedRegs))
		if c.regAlloc.nextReg > 20 { // Only warn if we have a lot of registers allocated
			fmt.Printf("// [REGALLOC WARNING] High register usage detected - potential leakage!\n")
		}
	}
	// <<< END ADDED >>>

	// <<< ADDED: Debug dump bytecode for functions >>>
	if debugCompiler && c.compilingFuncName != "<script>" && c.compilingFuncName != "" {
		// This is a function, dump its bytecode
		fmt.Printf("\n=== Function Bytecode: %s ===\n", c.compilingFuncName)
		fmt.Print(c.chunk.DisassembleChunk(c.compilingFuncName))
		fmt.Printf("=== END %s ===\n\n", c.compilingFuncName)
	}
	// <<< END ADDED >>>

	// Store the maximum registers needed for this chunk
	c.chunk.MaxRegs = int(c.regAlloc.MaxRegs())
	// Store spill slots needed for the main script/module chunk
	c.chunk.NumSpillSlots = int(c.nextSpillSlot)

	// Generate scope descriptor for module/script-level code if it contains direct eval
	// This is needed so that eval code can access local variables in the caller's scope
	if c.hasDirectEval && c.enclosing == nil {
		c.chunk.ScopeDesc = c.generateScopeDescriptor()
		debugPrintf("// [Compiler] Module/script has direct eval, generated scope descriptor with %d locals\n",
			len(c.chunk.ScopeDesc.LocalNames))
	}

	// Return the chunk (even if errors occurred, it might be partially useful for debugging?)
	// and the collected errors.
	return c.chunk, c.errors
}

// compileNode dispatches compilation to the appropriate method based on node type.
func (c *Compiler) compileNode(node parser.Node, hint Register) (Register, errors.PaseratiError) {
	// Safety check for nil checker, although it should be set by Compile()
	if c.typeChecker == nil && c.enclosing == nil { // Only panic if top-level compiler has no checker
		panic("Compiler internal error: typeChecker is nil during compileNode")
	}

	if c.line != parser.GetTokenFromNode(node).Line {
		c.line = parser.GetTokenFromNode(node).Line
		debugPrintf("// DEBUG compiling line %d (%s)\n", c.line, c.compilingFuncName)
	}

	// <<< ADDED: Enhanced node compilation logging >>>
	if debugCompiler {
		nodeType := fmt.Sprintf("%T", node)
		debugPrintf("// [NODE] Compiling %s at line %d, hint=R%d, nextReg=R%d\n",
			nodeType, c.line, hint, c.regAlloc.nextReg)
	}
	// <<< END ADDED >>>

	switch node := node.(type) {
	case *parser.Program:
		debugPrintf("// DEBUG Program: Starting statement loop.\n") // <<< ADDED
		hasResult := false                                          // Track whether any statement produced a value
		for i, stmt := range node.Statements {
			debugPrintf("// DEBUG Program: Before compiling statement %d (%T).\n", i, stmt) // <<< ADDED
			tlReg, err := c.compileNode(stmt, hint)
			if err != nil {
				debugPrintf("// DEBUG Program: Error compiling statement %d: %v\n", i, err) // <<< ADDED
				return BadRegister, err                                                     // Propagate errors up
			}

			// <<< ADDED vvv
			if c.enclosing == nil {
				debugPrintf("// DEBUG Program: After compiling statement %d (%T). Result: R%d\n", i, stmt, tlReg)
				// For top level, be conservative - don't free registers between statements
				// The VM will handle cleanup when the program ends
				// Track the most recent statement result to be the script's final result
				if tlReg != BadRegister {
					hint = tlReg
					hasResult = true
				}
			} else {
				// Inside function body - be more aggressive about freeing registers
				// But only free if we have more than a reasonable number allocated
				// DISABLED: Focus on expression-level freeing instead
				debugPrintf("// DEBUG Program: Inside function, but inter-statement freeing disabled\n")
			}
			// <<< ADDED ^^^
		}
		debugPrintf("// DEBUG Program: Finished statement loop. Final result: R%d, hasResult: %v\n", hint, hasResult) // <<< ADDED
		// If no statement produced a value, the hint register may contain stale data
		// (e.g., from hoisted functions). Return BadRegister to indicate undefined result.
		if !hasResult {
			return BadRegister, nil
		}
		return hint, nil // Return the last meaningful result

	// --- NEW: Handle Function Literal as an EXPRESSION first ---
	// This handles anonymous/named functions used in assignments, arguments, etc.
	// NOTE: Function declarations (standalone named functions) are handled by the ExpressionStatement
	// case below, not this case. This case handles function expressions in assignments, arguments, etc.
	case *parser.FunctionLiteral:
		debugPrintf("// DEBUG Node-FunctionLiteral: Compiling function literal used as expression '%s'.\n", node.Name) // <<< DEBUG

		// For function expressions, the nameHint should always be empty.
		// The function's internal name (for recursive calls from within) is handled by the
		// NFE (Named Function Expression) binding mechanism in compileFunctionLiteralWithOptions.
		// nameHint is only used for function declarations where the name is bound in outer scope.
		nameHint := ""
		// <<< MODIFY Call Site >>>
		funcConstIndex, freeSymbols, err := c.compileFunctionLiteral(node, nameHint)
		if err != nil {
			// Error already added to c.errors by compileFunctionLiteral
			return BadRegister, nil // Return nil error here, main error is tracked
		}

		// Ensure we have a valid destination register for the closure
		if hint == NoHint || hint == BadRegister {
			hint = c.regAlloc.Alloc()
		}
		// Emit OpClosure into the destination register
		c.emitClosure(hint, funcConstIndex, node, freeSymbols)

		// emitClosure now handles setting lastExprReg/Valid correctly.
		return hint, nil // <<< Return nil error

	// --- NEW: Handle Shorthand Method as an EXPRESSION ---
	// This handles shorthand methods in object literals like { method() { ... } }
	case *parser.ShorthandMethod:
		debugPrintf("// DEBUG Node-ShorthandMethod: Compiling shorthand method '%s'.\n", node.Name.Value)
		// Shorthand methods are essentially function expressions with a known name
		nameHint := ""
		if node.Name != nil {
			nameHint = node.Name.Value
		}

		funcConstIndex, freeSymbols, err := c.compileShorthandMethod(node, nameHint)
		if err != nil {
			return BadRegister, err
		}

		// Ensure destination register is valid
		if hint == NoHint || hint == BadRegister {
			hint = c.regAlloc.Alloc()
		}
		// Emit OpClosure using the generic emitter
		c.emitClosureGeneric(hint, funcConstIndex, node.Token.Line, node.Name, freeSymbols)

		return hint, nil

	// --- Block Statement (needed for function bodies) ---
	case *parser.BlockStatement:
		// Determine if this BlockStatement needs its own enclosed scope
		// - Function bodies: already have their own scope from newFunctionCompiler (isCompilingFunctionBody=true)
		// - All other blocks (top-level and inner): YES enclosed scope for proper let/const shadowing
		//
		// Rule: Create enclosed scope for all BlockStatements EXCEPT function body itself
		needsEnclosedScope := !c.isCompilingFunctionBody

		debugPrintf("// [BlockStatement] needsEnclosedScope=%v, enclosing=%v, isCompilingFunctionBody=%v\n",
			needsEnclosedScope, c.enclosing != nil, c.isCompilingFunctionBody)

		// Reset isCompilingFunctionBody after checking it, so inner BlockStatements are treated as regular blocks
		if c.isCompilingFunctionBody {
			c.isCompilingFunctionBody = false
		}

		// Create enclosed scope for inner blocks
		var prevSymbolTable *SymbolTable
		if needsEnclosedScope {
			prevSymbolTable = c.currentSymbolTable
			c.currentSymbolTable = NewEnclosedSymbolTable(c.currentSymbolTable)
			debugPrintf("// [BlockStatement] Created enclosed scope\n")
		}

		// 0) Predefine block-scoped let/const and function-scoped var so inner closures can capture stable locations
		// Reserve temp registers for spilling operations and general temp use
		// We reserve a pool to ensure temps are available throughout the hoisting phase
		const tempPoolSize = 16
		var tempPool []Register
		var spillTempReg Register
		spillTempUsed := false

		if len(node.Statements) > 0 || len(node.HoistedDeclarations) > 0 {
			tempPool = make([]Register, tempPoolSize)
			for i := 0; i < tempPoolSize; i++ {
				tempPool[i] = c.regAlloc.Alloc()
			}
			spillTempReg = tempPool[0] // Use first temp for spill operations
		}

		if len(node.Statements) > 0 {
			for _, stmt := range node.Statements {
				switch s := stmt.(type) {
				case *parser.LetStatement:
					if s.Name != nil {
						// Check if variable exists in CURRENT scope only (not outer scopes)
						// to allow shadowing in enclosed block scopes
						if _, alreadyInCurrentScope := c.currentSymbolTable.store[s.Name.Value]; !alreadyInCurrentScope {
							reg, ok := c.regAlloc.TryAllocForVariable()
							if ok {
								c.currentSymbolTable.DefineTDZ(s.Name.Value, reg)
								c.regAlloc.Pin(reg)
								// Emit TDZ marker (Uninitialized value) into the register
								c.emitLoadUninitialized(reg, s.Token.Line)
								debugPrintf("// [BlockPredefine] Pre-defined let '%s' in register R%d (symbolTable=%p)\n", s.Name.Value, reg, c.currentSymbolTable)
							} else {
								// Variable register threshold reached, use spilling
								spillIdx := c.AllocSpillSlot()
								c.currentSymbolTable.DefineTDZSpilled(s.Name.Value, spillIdx)
								// Emit TDZ marker to temp register, then store to spill slot
								tempReg := c.regAlloc.Alloc()
								c.emitLoadUninitialized(tempReg, s.Token.Line)
								c.emitStoreSpill(spillIdx, tempReg, s.Token.Line)
								c.regAlloc.Free(tempReg)
								debugPrintf("// [BlockPredefine] Pre-defined let '%s' in SPILL SLOT %d (symbolTable=%p)\n", s.Name.Value, spillIdx, c.currentSymbolTable)
							}
						}
					}
				case *parser.ConstStatement:
					if s.Name != nil {
						// Check if variable exists in CURRENT scope only (not outer scopes)
						// to allow shadowing in enclosed block scopes
						if _, alreadyInCurrentScope := c.currentSymbolTable.store[s.Name.Value]; !alreadyInCurrentScope {
							reg, ok := c.regAlloc.TryAllocForVariable()
							if ok {
								c.currentSymbolTable.DefineConstTDZ(s.Name.Value, reg)
								c.regAlloc.Pin(reg)
								// Emit TDZ marker (Uninitialized value) into the register
								c.emitLoadUninitialized(reg, s.Token.Line)
								debugPrintf("// [BlockPredefine] Pre-defined const '%s' in register R%d (symbolTable=%p)\n", s.Name.Value, reg, c.currentSymbolTable)
							} else {
								// Variable register threshold reached, use spilling
								spillIdx := c.AllocSpillSlot()
								c.currentSymbolTable.DefineConstTDZSpilled(s.Name.Value, spillIdx)
								// Emit TDZ marker to temp register, then store to spill slot
								tempReg := c.regAlloc.Alloc()
								c.emitLoadUninitialized(tempReg, s.Token.Line)
								c.emitStoreSpill(spillIdx, tempReg, s.Token.Line)
								c.regAlloc.Free(tempReg)
								debugPrintf("// [BlockPredefine] Pre-defined const '%s' in SPILL SLOT %d (symbolTable=%p)\n", s.Name.Value, spillIdx, c.currentSymbolTable)
							}
						}
					}
				case *parser.ObjectDestructuringDeclaration:
					// Pre-define all variables from object destructuring pattern
					// This is needed so closures can capture them before the destructuring is executed
					// Use TDZ for let/const, regular Define for var
					useTDZ := s.Token.Type == lexer.LET || s.Token.Type == lexer.CONST
					isConst := s.Token.Type == lexer.CONST
					varNames := extractDestructuringVarNames(s)
					for _, name := range varNames {
						if _, alreadyInCurrentScope := c.currentSymbolTable.store[name]; !alreadyInCurrentScope {
							reg, ok := c.regAlloc.TryAllocForVariable()
							if ok {
								if useTDZ {
									if isConst {
										c.currentSymbolTable.DefineConstTDZ(name, reg)
									} else {
										c.currentSymbolTable.DefineTDZ(name, reg)
									}
									// Emit TDZ marker (Uninitialized value) into the register
									c.emitLoadUninitialized(reg, s.Token.Line)
								} else {
									c.currentSymbolTable.Define(name, reg)
								}
								c.regAlloc.Pin(reg)
								debugPrintf("// [BlockPredefine] Pre-defined destructured '%s' in register R%d (TDZ=%v) (symbolTable=%p)\n", name, reg, useTDZ, c.currentSymbolTable)
							} else {
								// Variable register threshold reached, use spilling
								spillIdx := c.AllocSpillSlot()
								if useTDZ {
									if isConst {
										c.currentSymbolTable.DefineConstTDZSpilled(name, spillIdx)
									} else {
										c.currentSymbolTable.DefineTDZSpilled(name, spillIdx)
									}
									// Emit TDZ marker to temp register, then store to spill slot
									tempReg := c.regAlloc.Alloc()
									c.emitLoadUninitialized(tempReg, s.Token.Line)
									c.emitStoreSpill(spillIdx, tempReg, s.Token.Line)
									c.regAlloc.Free(tempReg)
								} else {
									c.currentSymbolTable.DefineSpilled(name, spillIdx)
								}
								debugPrintf("// [BlockPredefine] Pre-defined destructured '%s' in SPILL SLOT %d (TDZ=%v) (symbolTable=%p)\n", name, spillIdx, useTDZ, c.currentSymbolTable)
							}
						}
					}
				case *parser.ArrayDestructuringDeclaration:
					// Pre-define all variables from array destructuring pattern
					// Use TDZ for let/const, regular Define for var
					useTDZ := s.Token.Type == lexer.LET || s.Token.Type == lexer.CONST
					isConst := s.Token.Type == lexer.CONST
					varNames := extractArrayDestructuringVarNames(s)
					for _, name := range varNames {
						if _, alreadyInCurrentScope := c.currentSymbolTable.store[name]; !alreadyInCurrentScope {
							reg, ok := c.regAlloc.TryAllocForVariable()
							if ok {
								if useTDZ {
									if isConst {
										c.currentSymbolTable.DefineConstTDZ(name, reg)
									} else {
										c.currentSymbolTable.DefineTDZ(name, reg)
									}
									// Emit TDZ marker (Uninitialized value) into the register
									c.emitLoadUninitialized(reg, s.Token.Line)
								} else {
									c.currentSymbolTable.Define(name, reg)
								}
								c.regAlloc.Pin(reg)
								debugPrintf("// [BlockPredefine] Pre-defined array destructured '%s' in register R%d (TDZ=%v) (symbolTable=%p)\n", name, reg, useTDZ, c.currentSymbolTable)
							} else {
								// Variable register threshold reached, use spilling
								spillIdx := c.AllocSpillSlot()
								if useTDZ {
									if isConst {
										c.currentSymbolTable.DefineConstTDZSpilled(name, spillIdx)
									} else {
										c.currentSymbolTable.DefineTDZSpilled(name, spillIdx)
									}
									// Emit TDZ marker to temp register, then store to spill slot
									tempReg := c.regAlloc.Alloc()
									c.emitLoadUninitialized(tempReg, s.Token.Line)
									c.emitStoreSpill(spillIdx, tempReg, s.Token.Line)
									c.regAlloc.Free(tempReg)
								} else {
									c.currentSymbolTable.DefineSpilled(name, spillIdx)
								}
								debugPrintf("// [BlockPredefine] Pre-defined array destructured '%s' in SPILL SLOT %d (TDZ=%v) (symbolTable=%p)\n", name, spillIdx, useTDZ, c.currentSymbolTable)
							}
						}
					}
				case *parser.VarStatement:
					// Pre-define var declarations in the function scope (not enclosed block scope)
					// var is function-scoped, so if we're in an enclosed block scope, we need to
					// walk up to find the function-level symbol table
					for _, declarator := range s.Declarations {
						if declarator.Name != nil {
							// Find the function-level symbol table (the one with Outer == nil OR whose Outer is from enclosing compiler)
							funcTable := c.currentSymbolTable
							for funcTable.Outer != nil {
								// Stop if the outer scope belongs to an enclosing compiler (for nested functions)
								if c.enclosing != nil && c.isDefinedInEnclosingCompiler(funcTable.Outer) {
									break
								}
								funcTable = funcTable.Outer
							}

							// Check if var already defined in function scope
							if sym, _, found := funcTable.Resolve(declarator.Name.Value); !found || sym.Register == nilRegister {
								reg, ok := c.regAlloc.TryAllocForVariable()
								if ok {
									funcTable.Define(declarator.Name.Value, reg)
									// Smart pinning: Don't pin here - register will be pinned when/if captured by inner closure
									// Special case: in non-strict mode, `var arguments` is initialized to the Arguments object
									if declarator.Name.Value == "arguments" && c.isFunctionBody && !c.isArrowFunction && !c.chunk.IsStrict {
										c.emitGetArguments(reg, s.Token.Line)
									} else {
										// Initialize to undefined (var hoisting semantics: declaration is hoisted, not initialization)
										c.emitLoadUndefined(reg, s.Token.Line)
									}
									debugPrintf("// [BlockPredefine] Pre-defined var '%s' in register R%d in function scope (funcTable=%p, currentTable=%p)\n", declarator.Name.Value, reg, funcTable, c.currentSymbolTable)
								} else {
									// Variable register threshold reached, use spilling
									spillIdx := c.AllocSpillSlot()
									funcTable.DefineSpilled(declarator.Name.Value, spillIdx)
									// Initialize spill slot
									spillTempUsed = true
									// Special case: in non-strict mode, `var arguments` is initialized to the Arguments object
									if declarator.Name.Value == "arguments" && c.isFunctionBody && !c.isArrowFunction && !c.chunk.IsStrict {
										c.emitGetArguments(spillTempReg, s.Token.Line)
									} else {
										// Initialize to undefined using the pre-reserved spillTempReg
										c.emitLoadUndefined(spillTempReg, s.Token.Line)
									}
									c.emitStoreSpill(spillIdx, spillTempReg, s.Token.Line)
									debugPrintf("// [BlockPredefine] Pre-defined var '%s' in SPILL SLOT %d in function scope (funcTable=%p, currentTable=%p)\n", declarator.Name.Value, spillIdx, funcTable, c.currentSymbolTable)
								}
							}
						}
					}
				}
			}
		}

		// Free temp pool (except spillTempReg) BEFORE processing hoisted functions
		// This makes registers available for capturing spilled variables in closures
		// We keep spillTempReg for emitting spilled hoisted functions
		for i := 1; i < len(tempPool); i++ {
			c.regAlloc.Free(tempPool[i])
		}

		// 0.5) Hoist var declarations within this function body FIRST
		// var declarations are hoisted to the top of the function scope and initialized to undefined
		// This must happen BEFORE function hoisting so that functions can capture hoisted vars
		//
		// IMPORTANT: Only do this for function bodies (!needsEnclosedScope), not nested blocks.
		// For nested blocks at module level, vars are already hoisted by module-level hoisting.
		// For nested blocks in functions, vars are already hoisted by the function body.
		varNames := collectVarDeclarations(node.Statements)
		if !needsEnclosedScope {
			// This is a function body - hoist vars here
			for _, name := range varNames {
				// Skip if already defined in CURRENT scope (e.g., by a parameter)
				// Don't use Resolve() here - it traverses parent scopes and would skip
				// local vars that shadow globals. We only want to skip actual duplicates
				// in the current function scope (like a parameter with the same name).
				if _, alreadyInCurrentScope := c.currentSymbolTable.store[name]; alreadyInCurrentScope {
					continue
				}
			// Allocate register and define the variable, initialize to undefined
			reg, ok := c.regAlloc.TryAllocForVariable()
			if ok {
				c.currentSymbolTable.Define(name, reg)
				// Special case: in non-strict mode, `var arguments` is initialized to the Arguments object
				// Per ECMAScript spec, the arguments binding is initialized to the arguments object
				if name == "arguments" && c.isFunctionBody && !c.isArrowFunction && !c.chunk.IsStrict {
					c.emitGetArguments(reg, node.Token.Line)
					debugPrintf("// [VarHoist] Hoisted var 'arguments' in R%d (initialized to Arguments object)\n", reg)
				} else {
					// Initialize the register to undefined (hoisted vars start as undefined)
					c.emitLoadUndefined(reg, node.Token.Line)
					debugPrintf("// [VarHoist] Hoisted var '%s' in R%d (initialized to undefined)\n", name, reg)
				}
				// Don't pin - let smart pinning handle it when/if captured
			} else {
				// Variable threshold reached, use spilling
				spillIdx := c.AllocSpillSlot()
				c.currentSymbolTable.DefineSpilled(name, spillIdx)
				// Special case: in non-strict mode, `var arguments` is initialized to the Arguments object
				tempReg := c.regAlloc.Alloc()
				if name == "arguments" && c.isFunctionBody && !c.isArrowFunction && !c.chunk.IsStrict {
					c.emitGetArguments(tempReg, node.Token.Line)
					debugPrintf("// [VarHoist] Hoisted var 'arguments' in SPILL SLOT %d (initialized to Arguments object)\n", spillIdx)
				} else {
					// Initialize spill slot to undefined
					c.emitLoadUndefined(tempReg, node.Token.Line)
					debugPrintf("// [VarHoist] Hoisted var '%s' in SPILL SLOT %d (initialized to undefined)\n", name, spillIdx)
				}
				c.emitStoreSpill(spillIdx, tempReg, node.Token.Line)
				c.regAlloc.Free(tempReg)
			}
		}
		} // end if !needsEnclosedScope (function body var hoisting)

		// 1) Hoist function declarations within this block (function-scoped hoisting)
		if len(node.HoistedDeclarations) > 0 {
			debugPrintf("// [BlockStatement] Processing %d hoisted declarations\n", len(node.HoistedDeclarations))
			// Sort hoisted function names for deterministic compilation order
			// Go maps have non-deterministic iteration order, which can cause
			// different register/spill slot assignments between runs
			hoistedNames := make([]string, 0, len(node.HoistedDeclarations))
			for name := range node.HoistedDeclarations {
				hoistedNames = append(hoistedNames, name)
			}
			sort.Strings(hoistedNames)
			// Pre-allocate registers (or spill slots) for all hoisted function names to enable mutual recursion with stable locations
			for _, name := range hoistedNames {
				if sym, _, found := c.currentSymbolTable.Resolve(name); !found || sym.Register == nilRegister {
					reg, ok := c.regAlloc.TryAllocForVariable()
					if ok {
						c.currentSymbolTable.Define(name, reg)
						c.regAlloc.Pin(reg)
						debugPrintf("// [HoistedFunc] Pre-defined function '%s' in register R%d\n", name, reg)
					} else {
						// Variable register threshold reached, use spilling for hoisted function
						spillIdx := c.AllocSpillSlot()
						c.currentSymbolTable.DefineSpilled(name, spillIdx)
						debugPrintf("// [HoistedFunc] Pre-defined function '%s' in SPILL SLOT %d\n", name, spillIdx)
					}
				}
			}
			// Compile each hoisted function and emit its closure into the preallocated register or spill slot
			for _, name := range hoistedNames {
				decl := node.HoistedDeclarations[name]
				if funcLit, ok := decl.(*parser.FunctionLiteral); ok {
					funcConstIndex, freeSymbols, err := c.compileFunctionLiteral(funcLit, name)
					if err != nil {
						return BadRegister, err
					}
					// Get the preallocated location for this name
					sym, _, _ := c.currentSymbolTable.Resolve(name)
					if sym.IsSpilled {
						// Emit closure into temp register, then store to spill slot
						spillTempUsed = true
						c.emitClosure(spillTempReg, funcConstIndex, funcLit, freeSymbols)
						c.emitStoreSpill(sym.SpillIndex, spillTempReg, funcLit.Token.Line)
						debugPrintf("// [HoistedFunc] Emitted closure for '%s' to spill slot %d via temp R%d\n", name, sym.SpillIndex, spillTempReg)
					} else {
						bindingReg := sym.Register
						c.emitClosure(bindingReg, funcConstIndex, funcLit, freeSymbols)
						debugPrintf("// [HoistedFunc] Emitted closure for '%s' to register R%d\n", name, bindingReg)
					}
				}
			}
		}

		// Free spillTempReg (the remaining temp pool register)
		_ = spillTempUsed // Suppress unused warning
		_ = spillTempReg  // Suppress unused warning (it's tempPool[0])
		if len(tempPool) > 0 {
			c.regAlloc.Free(tempPool[0])
		}

		// 2) Compile statements in order, tracking completion value
		// Per ECMAScript, block completion value is the value of the last statement that produces a value
		// Compile directly with hint so nested try-finally with break/continue can correctly
		// propagate completion values
		hasCompletionValue := false
		for stmtIdx, stmt := range node.Statements {
			_ = stmtIdx // Suppress unused warning
			debugPrintf("// [BlockStatement] Compiling statement %d/%d: %T\n", stmtIdx, len(node.Statements), stmt)
			resultReg, err := c.compileNode(stmt, hint)
			if err != nil {
				debugPrintf("// [BlockStatement] ERROR at statement %d: %v\n", stmtIdx, err)
				return BadRegister, err
			}
			// If the statement produced a value, it's already in hint
			if resultReg != BadRegister {
				hasCompletionValue = true
			}
		}

		// Restore previous scope if we created an enclosed one
		if needsEnclosedScope {
			c.currentSymbolTable = prevSymbolTable
			debugPrintf("// [BlockStatement] Restored previous scope\n")
		}

		// Return hint if we got a completion value, otherwise BadRegister
		if hasCompletionValue {
			return hint, nil
		}
		return BadRegister, nil

	// --- Statements ---
	case *parser.TypeAliasStatement: // Added
		// Type aliases only exist for type checking, ignore in compiler.
		return BadRegister, nil

	case *parser.InterfaceDeclaration: // Added
		// Interface declarations only exist for type checking, ignore in compiler.
		return BadRegister, nil

	case *parser.FunctionSignature: // Added

		return BadRegister, nil

	case *parser.ExpressionStatement:
		debugPrintf("// DEBUG ExprStmt: Compiling expression %T.\n", node.Expression)

		// Check specifically for NAMED function literals used as standalone statements.
		// Anonymous ones are handled by the case *parser.FunctionLiteral above now.
		if funcLit, ok := node.Expression.(*parser.FunctionLiteral); ok && funcLit.Name != nil {
			debugPrintf("// DEBUG ExprStmt: Handling NAMED function declaration '%s' as statement.\n", funcLit.Name.Value)

			// Check if this function was already processed during hoisting
			// For global hoisted functions, IsGlobal is true (but Register may be nilRegister since it was freed)
			// For local hoisted functions, Register != nilRegister
			if symbolRef, _, found := c.currentSymbolTable.Resolve(funcLit.Name.Value); found && (symbolRef.Register != nilRegister || symbolRef.IsGlobal) {
				debugPrintf("// DEBUG ExprStmt: Function '%s' already hoisted, skipping duplicate processing.\n", funcLit.Name.Value)
				// Function was already hoisted and processed, skip it
				return BadRegister, nil
			}

			// --- Handle named function recursion ---
			// 1. Define the function name temporarily.
			c.currentSymbolTable.Define(funcLit.Name.Value, nilRegister)

			// 2. Compile the function literal body. Pass name as hint.
			// <<< MODIFY Call Site >>>
			funcConstIndex, freeSymbols, err := c.compileFunctionLiteral(funcLit, funcLit.Name.Value) // Use its own name as hint
			if err != nil {
				// Error already added to c.errors by compileFunctionLiteral
				return BadRegister, nil // Return nil error here, main error is tracked
			}

			// 3. Create the closure object in a dedicated persistent register and pin it
			bindingReg := c.regAlloc.Alloc()
			c.emitClosure(bindingReg, funcConstIndex, funcLit, freeSymbols)
			c.regAlloc.Pin(bindingReg)

			// 4. Update the symbol table entry with the register holding the closure.
			c.currentSymbolTable.UpdateRegister(funcLit.Name.Value, bindingReg)

			// 5. If at top level, also set as global variable
			if c.enclosing == nil {
				globalIdx := c.GetOrAssignGlobalIndex(funcLit.Name.Value)
				c.emitSetGlobal(globalIdx, bindingReg, funcLit.Token.Line)
				c.currentSymbolTable.DefineGlobal(funcLit.Name.Value, globalIdx)
				// Mark as non-configurable (DontDelete) only for true top-level function declarations,
				// not for eval-created bindings which should be configurable
				if !c.isIndirectEval && c.callerScopeDesc == nil {
					c.MarkVarGlobal(globalIdx)
				}
			}

			// Function declarations don't produce a value for the script result
			return BadRegister, nil // Handled
		}

		// Original ExpressionStatement logic for other expressions
		debugPrintf("// DEBUG ExprStmt: Compiling non-func-decl expression %T.\n", node.Expression)
		hint, err := c.compileNode(node.Expression, hint)
		if err != nil {
			return BadRegister, err
		}
		debugPrintf("// DEBUG ExprStmt: Top Level. CurrentReg: R%d.\n", hint) // <<< ADDED
		// Result register is left allocated, potentially unused otherwise.
		// TODO: Consider freeing registers?
		return hint, nil // ADDED: Explicit return

	case *parser.ClassDeclaration:
		return c.compileClassDeclaration(node, hint)

	case *parser.EnumDeclaration:
		return c.compileEnumDeclaration(node, hint)

	case *parser.ClassExpression:
		// Convert ClassExpression to ClassDeclaration for compilation
		// The compiler treats them the same way
		var className *parser.Identifier
		if node.Name == nil {
			// Generate a unique name for anonymous classes
			// This follows the pattern used for anonymous functions
			anonymousName := fmt.Sprintf("__AnonymousClass_%d", c.getNextAnonymousId())
			className = &parser.Identifier{
				Token: node.Token,
				Value: anonymousName,
			}
		} else {
			className = node.Name
		}

		classDecl := &parser.ClassDeclaration{
			Token:          node.Token,
			Name:           className,
			TypeParameters: node.TypeParameters,
			SuperClass:     node.SuperClass,
			Implements:     node.Implements,
			Body:           node.Body,
			IsAbstract:     node.IsAbstract,
		}
		// For class expressions, we need to return the constructor function
		// instead of just defining it in the environment like class declarations
		return c.compileClassExpression(classDecl, hint)

	case *parser.LetStatement:
		return c.compileLetStatement(node, hint) // TODO: Fix this

	case *parser.VarStatement:
		return c.compileVarStatement(node, hint)

	case *parser.ConstStatement:
		return c.compileConstStatement(node, hint) // TODO: Fix this

	case *parser.ArrayDestructuringDeclaration:
		return c.compileArrayDestructuringDeclaration(node, hint)

	case *parser.ObjectDestructuringDeclaration:
		return c.compileObjectDestructuringDeclaration(node, hint)

	case *parser.ReturnStatement: // Although less relevant for top-level script return
		return c.compileReturnStatement(node, hint) // TODO: Fix this

	case *parser.WhileStatement:
		return c.compileWhileStatement(node, hint) // TODO: Fix this

	case *parser.ForStatement:
		return c.compileForStatement(node, hint) // TODO: Fix this

	case *parser.ForOfStatement:
		return c.compileForOfStatement(node, hint) // TODO: Fix this

	case *parser.ForInStatement:
		return c.compileForInStatement(node, hint)

	case *parser.BreakStatement:
		return c.compileBreakStatement(node, hint) // TODO: Fix this

	case *parser.EmptyStatement:
		return c.compileEmptyStatement(node, hint)
	case *parser.ContinueStatement:
		return c.compileContinueStatement(node, hint) // TODO: Fix this

	case *parser.LabeledStatement:
		return c.compileLabeledStatement(node, hint)

	case *parser.DoWhileStatement:
		return c.compileDoWhileStatement(node, hint) // TODO: Fix this

	case *parser.SwitchStatement: // Added
		return c.compileSwitchStatement(node, hint) // TODO: Fix this

	// --- Exception Handling Statements ---
	case *parser.TryStatement:
		return c.compileTryStatement(node, hint)

	case *parser.ThrowStatement:
		return c.compileThrowStatement(node, hint)

	case *parser.DebuggerStatement:
		// debugger statement is a no-op at runtime
		return c.compileDebuggerStatement(hint)

	case *parser.WithStatement:
		return c.compileWithStatement(node, hint)

	// --- Module Statements ---
	case *parser.ImportDeclaration:
		return c.compileImportDeclaration(node, hint)

	case *parser.ExportNamedDeclaration:
		return c.compileExportNamedDeclaration(node, hint)

	case *parser.ExportDefaultDeclaration:
		return c.compileExportDefaultDeclaration(node, hint)

	case *parser.ExportAllDeclaration:
		return c.compileExportAllDeclaration(node, hint)

	// --- Expressions (excluding FunctionLiteral which is handled above) ---
	case *parser.NumberLiteral:
		//fmt.Printf("[NUMBER LITERAL DEBUG] Compiling NumberLiteral value=%f with hint=R%d\n", node.Value, hint)
		c.emitLoadNewConstant(hint, vm.Number(node.Value), node.Token.Line)
		return hint, nil // ADDED: Explicit return

	case *parser.BigIntLiteral:
		// Parse the numeric part and create a big.Int
		bigIntValue := new(big.Int)
		if _, ok := bigIntValue.SetString(node.Value, 0); !ok {
			return BadRegister, NewCompileError(node, fmt.Sprintf("invalid BigInt literal: %s", node.Value))
		}
		c.emitLoadNewConstant(hint, vm.NewBigInt(bigIntValue), node.Token.Line)
		return hint, nil

	case *parser.StringLiteral:
		// Handle string literals by adding them to constants
		c.emitLoadNewConstant(hint, vm.String(node.Value), node.Token.Line)
		return hint, nil

	case *parser.TemplateLiteral:
		return c.compileTemplateLiteral(node, hint) // TODO: Fix this

	case *parser.TaggedTemplateExpression:
		return c.compileTaggedTemplate(node, hint)

	case *parser.BooleanLiteral:
		// Handle boolean literals by using appropriate opcode
		if node.Value {
			c.emitLoadTrue(hint, node.Token.Line)
		} else {
			c.emitLoadFalse(hint, node.Token.Line)
		}
		return hint, nil

	case *parser.NullLiteral:
		c.emitLoadNull(hint, node.Token.Line)
		return hint, nil // ADDED: Explicit return

	case *parser.UndefinedLiteral: // Added
		c.emitLoadUndefined(hint, node.Token.Line)
		return hint, nil // ADDED: Explicit return

	case *parser.RegexLiteral: // Added for regex literals
		// Per ECMAScript spec, each evaluation of a RegExp literal creates a NEW object
		// We emit OpMakeRegExp which creates a new RegExp at runtime from pattern and flags
		patternIdx := c.chunk.AddConstant(vm.String(node.Pattern))
		flagsIdx := c.chunk.AddConstant(vm.String(node.Flags))
		c.emitMakeRegExp(hint, patternIdx, flagsIdx, node.Token.Line)
		return hint, nil

	case *parser.ThisExpression: // Added for this keyword
		// Load 'this' value from current call context
		c.emitLoadThis(hint, node.Token.Line)
		return hint, nil

	case *parser.SuperExpression:
		// Super should only appear in member expressions or call expressions
		// For now, keep the OpLoadSuper emission for edge cases
		// TODO: Investigate if this code path is ever actually reached
		c.chunk.WriteOpCode(vm.OpLoadSuper, node.Token.Line)
		c.chunk.EmitByte(byte(hint))
		return hint, nil

	case *parser.NewTargetExpression:
		// Per ECMAScript, new.target is only valid inside functions.
		// In indirect eval (evaluated in global scope without a caller), it's a SyntaxError.
		// Check: isIndirectEval + no callerScopeDesc (no function context to inherit from) + no newTargetAvailable
		// For direct eval, callerScopeDesc provides the enclosing function context.
		// For arrow functions in indirect eval, they inherit newTargetAvailable from enclosing scope.
		if c.isIndirectEval && c.callerScopeDesc == nil && !c.newTargetAvailable {
			return BadRegister, NewCompileError(node, "new.target expression is not allowed in indirect eval")
		}
		// Load new.target value from constructor context
		c.emitLoadNewTarget(hint, node.Token.Line)
		return hint, nil

	case *parser.ImportMetaExpression:
		// import.meta is only valid in module code (ECMAScript spec)
		// Also check forceScriptMode which is set by Function() constructor
		if !c.IsModuleMode() || c.forceScriptMode {
			err := NewCompileError(node, "'import.meta' is only valid in module code")
			c.errors = append(c.errors, err) // Also add to errors list for proper propagation
			return BadRegister, err
		}
		// Load import.meta value from module context
		c.emitLoadImportMeta(hint, node.Token.Line)
		return hint, nil

	case *parser.DynamicImportExpression:
		// Compile the module specifier expression
		specifierReg := c.regAlloc.Alloc()
		defer c.regAlloc.Free(specifierReg)

		specifierReg, err := c.compileNode(node.Source, specifierReg)
		if err != nil {
			return nilRegister, err
		}

		// Emit dynamic import instruction
		// For now, this will load the module synchronously (simplified implementation)
		// TODO: Implement proper Promise-based async loading
		c.emitDynamicImport(hint, specifierReg, node.Token.Line)
		return hint, nil

	case *parser.DeferredImportExpression:
		// Compile the module specifier expression
		specifierReg := c.regAlloc.Alloc()
		defer c.regAlloc.Free(specifierReg)

		specifierReg, err := c.compileNode(node.Source, specifierReg)
		if err != nil {
			return nilRegister, err
		}

		// Emit deferred import instruction
		// For now, treat it the same as dynamic import (simplified implementation)
		// TODO: Implement proper deferred loading semantics
		c.emitDynamicImport(hint, specifierReg, node.Token.Line)
		return hint, nil

	case *parser.PrivateIdentifier:
		// PrivateIdentifier is only valid as the left operand of 'in' expressions (#field in obj)
		// If we get here, it's being used in an invalid context
		return BadRegister, NewCompileError(node,
			fmt.Sprintf("Private identifier '%s' can only be used on the left side of 'in' expressions", node.Value))

	case *parser.Identifier:
		// Special handling for 'arguments' identifier - only available in non-arrow functions
		// BUT only if there's no user-defined variable named 'arguments' that shadows it
		if node.Value == "arguments" {
			// Per ECMAScript "Additional Early Error Rules for Eval Inside Initializer":
			// Accessing 'arguments' in eval inside class field initializer is a SyntaxError
			// This applies regardless of whether a user-defined 'arguments' variable exists
			// Check both direct callerScopeDesc and inherited isClassFieldInitializer flag
			if c.isClassFieldInitializer || (c.callerScopeDesc != nil && c.callerScopeDesc.InClassFieldInitializer) {
				return BadRegister, NewCompileError(node,
					"'arguments' is not allowed in class field initializer")
			}
			if c.isFunctionBody && !c.isArrowFunction {
				// We're in a regular function - check for shadowing WITHIN this function's scope only
				// (outer function's `var arguments` does NOT shadow this function's implicit binding)
				_, _, foundUserVar := c.currentSymbolTable.ResolveUpTo("arguments", c.scopeBoundary)
				if !foundUserVar {
					// No shadowing within this function - use its implicit arguments binding
					c.emitGetArguments(hint, node.Token.Line)
					return hint, nil
				}
				// User has defined 'arguments' in this function - fall through to normal variable lookup
			} else if c.isArrowFunction {
				// Arrow functions don't have own arguments, but capture from enclosing non-arrow function
				// First check if there's a user-defined 'arguments' variable anywhere
				_, _, foundUserVar := c.currentSymbolTable.Resolve("arguments")
				if !foundUserVar {
					// No user variable - walk up enclosing compilers to find a regular function
					for enc := c.enclosing; enc != nil; enc = enc.enclosing {
						if enc.isFunctionBody && !enc.isArrowFunction {
							// Found an enclosing regular function - use OpGetArguments
							// (this works because arguments is lazily created and shared)
							c.emitGetArguments(hint, node.Token.Line)
							return hint, nil
						}
					}
				}
				// User defined 'arguments' or no enclosing regular function - fall through to normal lookup
			}
			// If at module scope, treat as regular identifier
		}

		// Check for TDZ violation in default parameter expressions
		// When compiling a default value for parameter at index N, parameters at index >= N
		// are in the Temporal Dead Zone and must throw ReferenceError if accessed
		if c.inDefaultParamScope && c.currentDefaultParamIndex >= 0 && c.parameterList != nil {
			for i := c.currentDefaultParamIndex; i < len(c.parameterList); i++ {
				if c.parameterList[i] == node.Value {
					// Forward reference to uninitialized parameter - emit ReferenceError
					c.emitTDZError(hint, node.Value, node.Token.Line)
					return hint, nil
				}
			}
		}

		// All identifiers (including builtins) now use standard variable lookup
		// Builtins are registered as global variables via the new initializer system
		scopeName := "Function"
		if c.currentSymbolTable.Outer == nil {
			scopeName = "Global"
		}
		debugPrintf("// DEBUG Identifier '%s': Looking up in %s scope\n", node.Value, scopeName) // <<< ADDED
		symbolRef, definingTable, found := c.currentSymbolTable.Resolve(node.Value)

		// Note: TDZ (Temporal Dead Zone) checking is a runtime property that requires tracking
		// variable initialization state at runtime. Compile-time TDZ checking doesn't work correctly
		// because at compile time we don't know the order of code execution.
		// Proper TDZ implementation would require:
		// 1. A runtime "uninitialized" marker value for let/const variables
		// 2. Runtime checks before variable access
		// 3. Marking variables as initialized when their declaration is executed
		// For now, TDZ checking is disabled to avoid false positives with upvalues.

		if !found {
			debugPrintf("// DEBUG Identifier '%s': NOT FOUND in symbol table, checking with objects\n", node.Value) // <<< ADDED

			// Check caller scope first (for direct eval with scope access)
			if c.callerScopeDesc != nil {
				if callerRegIdx := c.resolveCallerLocal(node.Value); callerRegIdx >= 0 {
					debugPrintf("// DEBUG Identifier '%s': Found in caller scope at CallerR%d\n", node.Value, callerRegIdx)
					c.emitOpCode(vm.OpGetCallerLocal, node.Token.Line)
					c.emitByte(byte(hint))
					c.emitByte(byte(callerRegIdx))
					return hint, nil
				}
			}

			// Check if it's from a with object (flagged by type checker)
			if _, isWithProperty := c.shouldUseWithProperty(node); isWithProperty {
				debugPrintf("// DEBUG Identifier '%s': Found in with object, emitting OpGetWithProperty\n", node.Value)
				// Emit OpGetWithProperty to check with-object stack at runtime
				propNameIdx := c.chunk.AddConstant(vm.String(node.Value))
				c.emitGetWithProperty(hint, int(propNameIdx), node.Token.Line)
				return hint, nil
			}

			debugPrintf("// DEBUG Identifier '%s': NOT FOUND in symbol table or with objects, treating as GLOBAL\n", node.Value) // <<< ADDED

			// Check if this is an imported identifier that needs module evaluation first
			isModuleMode := c.IsModuleMode()
			isImported := c.moduleBindings != nil && c.moduleBindings.IsImported(node.Value)
			debugPrintf("// DEBUG Identifier '%s': IsModuleMode=%v, IsImported=%v (global path)\n", node.Value, isModuleMode, isImported)

			if isModuleMode && isImported {
				debugPrintf("// DEBUG Identifier '%s': This is an imported name, generating runtime import resolution (global path)\n", node.Value)
				// Generate code to resolve the import at runtime
				c.emitImportResolve(hint, node.Value, node.Token.Line)
				return hint, nil
			}

			// Variable not found in any scope, treat as a global variable access
			// This will return undefined at runtime if the global doesn't exist
			globalIdx := c.GetOrAssignGlobalIndex(node.Value)
			c.emitGetGlobal(hint, globalIdx, node.Token.Line)
			return hint, nil // Handle as global access
		}
		isLocal := definingTable == c.currentSymbolTable
		debugPrintf("// DEBUG Identifier '%s': Found in symbol table, isLocal=%v, definingTable==%p, currentTable==%p\n", node.Value, isLocal, definingTable, c.currentSymbolTable) // <<< ADDED

		// Handle caller local symbols (for direct eval with nested functions)
		// These symbols represent variables from the caller's scope that need to be accessed via OpGetCallerLocal
		if symbolRef.IsCallerLocal {
			debugPrintf("// DEBUG Identifier '%s': Found CALLER LOCAL at index %d, register R%d\n", node.Value, symbolRef.CallerLocalIndex, symbolRef.Register)
			// If we're in the eval compiler (have callerScopeDesc), emit OpGetCallerLocal
			// Load into the symbol's register so upvalue capture works correctly
			if c.callerScopeDesc != nil {
				// Emit OpGetCallerLocal to load value into the symbol's register
				c.emitOpCode(vm.OpGetCallerLocal, node.Token.Line)
				c.emitByte(byte(symbolRef.Register))
				c.emitByte(byte(symbolRef.CallerLocalIndex))
				// Move to hint if different
				if symbolRef.Register != hint {
					c.emitMove(hint, symbolRef.Register, node.Token.Line)
				}
				return hint, nil
			}
			// If we're in a nested function within eval, we need to capture this as a free variable
			// The upvalue mechanism will resolve it through the closure chain
			if c.enclosing != nil {
				debugPrintf("// DEBUG Identifier '%s': Caller local in nested function, treating as free variable\n", node.Value)
				freeVarIndex := c.addFreeSymbol(node, &symbolRef)
				c.emitLoadFree(hint, freeVarIndex, node.Token.Line)
				return hint, nil
			}
		}

		// NOTE: In ECMAScript, with-object properties SHADOW local variables, not vice versa.
		// The with statement creates an object environment record that is checked FIRST.
		// When c.withBlockDepth > 0, we use OpGetWithOrLocal to check the with-object at
		// runtime before falling back to the local variable. This is handled in the
		// local variable handling sections below.

		// --- NEW RECURSION CHECK --- // Revised Check
		// Check if this is a recursive call identifier referencing the temp definition.
		isRecursiveSelfCall := isLocal &&
			symbolRef.Register == nilRegister && // Is it our temporary definition?
			scopeName == "Function" // Are we compiling inside a function? // Removed check against c.compilingFuncName

		if isRecursiveSelfCall {
			debugPrintf("// DEBUG Identifier '%s': Identified as RECURSIVE SELF CALL\n", node.Value) // <<< ADDED
			// Check if the recursive call is actually to a global variable
			// (happens in module mode where top-level let/const become globals)
			// Find the root compiler (module level)
			rootCompiler := c
			for rootCompiler.enclosing != nil {
				rootCompiler = rootCompiler.enclosing
			}
			// Check if symbol is being defined at module level (root has no enclosing)
			isModuleLevelDef := (rootCompiler == c.enclosing) && symbolRef.Register == nilRegister

			if symbolRef.IsGlobal || isModuleLevelDef {
				// This is a global recursive call - module mode top-level function
				// The variable will be stored as a global, so use OpGetGlobal
				debugPrintf("// DEBUG Identifier '%s': Recursive call to GLOBAL (IsGlobal=%v, isModuleLevelDef=%v), using OpGetGlobal\n", node.Value, symbolRef.IsGlobal, isModuleLevelDef)
				// Get or assign the global index
				globalIdx := c.GetOrAssignGlobalIndex(node.Value)
				c.emitGetGlobal(hint, globalIdx, node.Token.Line)
			} else {
				// True local recursive call - needs upvalue capture
				debugPrintf("// DEBUG Identifier '%s': Recursive call to LOCAL, using upvalue\n", node.Value)
				freeVarIndex := c.addFreeSymbol(node, &symbolRef)
				c.emitLoadFree(hint, freeVarIndex, node.Token.Line)
			}
		} else if symbolRef.IsGlobal {
			// This is a global variable
			// Use withBlockDepth (not currentFuncWithDepth) so closures inside with blocks
			// also check captured with-objects for globals
			if c.withBlockDepth > 0 {
				// Inside a with block (or closure defined in one), use OpGetWithProperty
				// which checks with-object stack first, then falls back to global
				debugPrintf("// DEBUG Identifier '%s': GLOBAL variable inside with block, using OpGetWithProperty\n", node.Value)
				propNameIdx := c.chunk.AddConstant(vm.String(node.Value))
				c.emitGetWithProperty(hint, int(propNameIdx), node.Token.Line)
			} else {
				debugPrintf("// DEBUG Identifier '%s': GLOBAL variable, using OpGetGlobal\n", node.Value)
				c.emitGetGlobal(hint, symbolRef.GlobalIndex, node.Token.Line)
			}
		} else if !isLocal {
			// Check if this is an imported identifier before treating as free variable
			isModuleMode := c.IsModuleMode()
			isImported := c.moduleBindings != nil && c.moduleBindings.IsImported(node.Value)
			debugPrintf("// DEBUG Identifier '%s': NOT LOCAL, IsModuleMode=%v, IsImported=%v\n", node.Value, isModuleMode, isImported)

			if isModuleMode && isImported {
				debugPrintf("// DEBUG Identifier '%s': This is an imported name, generating runtime import resolution (non-local path)\n", node.Value)
				// Generate code to resolve the import at runtime
				c.emitImportResolve(hint, node.Value, node.Token.Line)
			} else if symbolRef.IsGlobal {
				// This is a global variable (already has a global index), use OpGetGlobal
				debugPrintf("// DEBUG Identifier '%s': NOT LOCAL but is GLOBAL, using OpGetGlobal\n", node.Value)
				c.emitGetGlobal(hint, symbolRef.GlobalIndex, node.Token.Line)
			} else if symbolRef.Register == nilRegister {
				// This is a module-level variable that's being defined (let/const in module scope)
				// It will become a global, so use OpGetGlobal
				debugPrintf("// DEBUG Identifier '%s': NOT LOCAL, Register==nilRegister, treating as module-level global\n", node.Value)
				globalIdx := c.GetOrAssignGlobalIndex(node.Value)
				c.emitGetGlobal(hint, globalIdx, node.Token.Line)
			} else if c.enclosing != nil && c.isDefinedInEnclosingCompiler(definingTable) {
				debugPrintf("// DEBUG Identifier '%s': NOT LOCAL, defined in OUTER FUNCTION, treating as FREE VARIABLE\n", node.Value)
				// Variable is defined in an outer function's scope (true closure/upvalue case)
				// Use OpLoadFree to access it via closure mechanism
				freeVarIndex := c.addFreeSymbol(node, &symbolRef)
				c.emitLoadFree(hint, freeVarIndex, node.Token.Line)
			} else if symbolRef.IsSpilled {
				// Variable is spilled (stored in spill slot, not a register)
				debugPrintf("// DEBUG Identifier '%s': NOT LOCAL, SPILLED variable in outer block scope, spillIdx=%d\n", node.Value, symbolRef.SpillIndex)
				c.emitLoadSpill(hint, symbolRef.SpillIndex, node.Token.Line)
			} else if c.currentFuncWithDepth > 0 {
				// Inside a with block in the CURRENT function, with-object properties shadow outer block variables.
				// NOTE: We use currentFuncWithDepth (not withBlockDepth) because this is still within
				// the same function - outer block vars can be shadowed by the current function's with-object.
				debugPrintf("// DEBUG Identifier '%s': In with block (outer block scope), using OpGetWithOrLocal\n", node.Value)
				srcReg := symbolRef.Register
				nameIdx := c.chunk.AddConstant(vm.NewString(node.Value))
				c.emitGetWithOrLocal(hint, int(nameIdx), srcReg, node.Token.Line)
			} else {
				debugPrintf("// DEBUG Identifier '%s': NOT LOCAL, but in outer block scope of SAME function, using direct register access R%d\n", node.Value, symbolRef.Register)
				// Variable is defined in an outer block scope of the same function (or at top level)
				// Access it directly via its register, no closure needed
				srcReg := symbolRef.Register
				// TDZ check for let/const variables in outer block scope
				if symbolRef.IsTDZ {
					c.emitCheckUninitialized(srcReg, node.Token.Line)
				}
				if srcReg != hint {
					debugPrintf("// DEBUG Identifier '%s': About to emit Move R%d (dest), R%d (src)\n", node.Value, hint, srcReg)
					c.emitMove(hint, srcReg, node.Token.Line)
				}
				// If srcReg == hint, no move needed
			}
		} else if symbolRef.IsSpilled {
			// LOCAL spilled variable - load from spill slot
			debugPrintf("// DEBUG Identifier '%s': LOCAL SPILLED variable, spillIdx=%d\n", node.Value, symbolRef.SpillIndex)
			c.emitLoadSpill(hint, symbolRef.SpillIndex, node.Token.Line)
		} else {
			debugPrintf("// DEBUG Identifier '%s': LOCAL variable, register=R%d\n", node.Value, symbolRef.Register) // <<< ADDED
			// This is a standard local variable (handled by current stack frame)
			srcReg := symbolRef.Register
			debugPrintf("// DEBUG Identifier '%s': Resolved to isLocal=%v, srcReg=R%d\n", node.Value, isLocal, srcReg)
			if srcReg == nilRegister {
				// Check if this is an imported identifier
				isModuleMode := c.IsModuleMode()
				isImported := c.moduleBindings != nil && c.moduleBindings.IsImported(node.Value)
				debugPrintf("// DEBUG Identifier '%s': IsModuleMode=%v, IsImported=%v\n", node.Value, isModuleMode, isImported)
				if isModuleMode && isImported {
					debugPrintf("// DEBUG Identifier '%s': This is an imported name, generating runtime import resolution\n", node.Value)
					// Generate code to resolve the import at runtime
					c.emitImportResolve(hint, node.Value, node.Token.Line)
				} else {
					// This panic indicates an internal logic error, like trying to use a variable
					// during its temporary definition phase inappropriately.
					panic(fmt.Sprintf("compiler internal error: resolved local variable '%s' to nilRegister R%d unexpectedly at line %d", node.Value, srcReg, node.Token.Line))
				}
			} else if c.currentFuncWithDepth > 0 {
				// Inside a with block in the CURRENT function, with-object properties shadow local variables.
				// NOTE: We use currentFuncWithDepth (not withBlockDepth) because nested functions
				// have their own scope - their locals should NOT be shadowed by an enclosing with-object.
				// Use OpGetWithOrLocal to check with-object at runtime, falling back to local.
				debugPrintf("// DEBUG Identifier '%s': In with block (current func), using OpGetWithOrLocal\n", node.Value)
				nameIdx := c.chunk.AddConstant(vm.NewString(node.Value))
				c.emitGetWithOrLocal(hint, int(nameIdx), srcReg, node.Token.Line)
			} else {
				// TDZ check for let/const local variables
				if symbolRef.IsTDZ {
					c.emitCheckUninitialized(srcReg, node.Token.Line)
				}
				debugPrintf("// DEBUG Identifier '%s': About to emit Move R%d (dest), R%d (src)\n", node.Value, hint, srcReg)
				c.emitMove(hint, srcReg, node.Token.Line)
			}
		}
		return hint, nil // ADDED: Explicit return

	case *parser.PrefixExpression:
		return c.compilePrefixExpression(node, hint) // TODO: Fix this

	case *parser.TypeofExpression:
		return c.compileTypeofExpression(node, hint) // TODO: Fix this

	case *parser.TypeAssertionExpression:
		return c.compileTypeAssertionExpression(node, hint)

	case *parser.SatisfiesExpression:
		return c.compileSatisfiesExpression(node, hint)

	case *parser.NonNullExpression:
		return c.compileNonNullExpression(node, hint)

	case *parser.InfixExpression:
		return c.compileInfixExpression(node, hint) // TODO: Fix this

	case *parser.ArrowFunctionLiteral: // Keep this separate
		return c.compileArrowFunctionLiteral(node, hint) // TODO: Fix this

	case *parser.CallExpression:
		return c.compileCallExpression(node, hint) // TODO: Fix this

	case *parser.IfExpression:
		return c.compileIfExpression(node, hint) // TODO: Fix this

	case *parser.IfStatement:
		// Handle if statements - reuse IfExpression compilation
		// Per ECMAScript, if statements DO produce completion values for eval()
		// Convert IfStatement to IfExpression for compilation
		ifExpr := &parser.IfExpression{
			Token:       node.Token,
			Condition:   node.Condition,
			Consequence: node.Consequence,
			Alternative: node.Alternative,
		}
		return c.compileIfExpression(ifExpr, hint)

	case *parser.TernaryExpression:
		return c.compileTernaryExpression(node, hint) // TODO: Fix this

	case *parser.AssignmentExpression:
		return c.compileAssignmentExpression(node, hint) // TODO: Fix this

	case *parser.ArrayDestructuringAssignment:
		return c.compileArrayDestructuringAssignment(node, hint)

	case *parser.ObjectDestructuringAssignment:
		return c.compileObjectDestructuringAssignment(node, hint)

	case *parser.UpdateExpression:
		return c.compileUpdateExpression(node, hint) // TODO: Fix this

	// --- NEW: Array/Index ---
	case *parser.ArrayLiteral:
		return c.compileArrayLiteral(node, hint) // TODO: Fix this
	case *parser.ObjectLiteral: // <<< NEW
		return c.compileObjectLiteral(node, hint) // TODO: Fix this
	case *parser.IndexExpression:
		return c.compileIndexExpression(node, hint) // TODO: Fix this
		// --- End Array/Index ---

		// --- Member Expression ---
	case *parser.MemberExpression:
		return c.compileMemberExpression(node, hint) // TODO: Fix this
		// --- END Member Expression ---

		// --- Optional Chaining Expression ---
	case *parser.OptionalChainingExpression:
		return c.compileOptionalChainingExpression(node, hint) // TODO: Fix this

	case *parser.OptionalIndexExpression:
		return c.compileOptionalIndexExpression(node, hint)

	case *parser.OptionalCallExpression:
		return c.compileOptionalCallExpression(node, hint)
		// --- END Optional Chaining Expression ---

	case *parser.NewExpression:
		return c.compileNewExpression(node, hint) // TODO: Fix this

	// --- NEW: Rest Parameters and Spread Syntax ---
	case *parser.SpreadElement:
		// SpreadElement can appear in function calls - this should be handled there
		// If we reach here, it's likely used in an invalid context
		return BadRegister, NewCompileError(node, "spread syntax not supported in this context")

	case *parser.RestParameter:
		// RestParameter should only appear in function parameter lists
		// If we reach here, it's likely used in an invalid context
		return BadRegister, NewCompileError(node, "rest parameter syntax not supported in this context")
	// --- END NEW ---

	// --- Generator Support ---
	case *parser.YieldExpression:
		return c.compileYieldExpression(node, hint)
	// --- END Generator Support ---

	// --- Async/Await Support ---
	case *parser.AwaitExpression:
		return c.compileAwaitExpression(node, hint)
	// --- END Async/Await Support ---

	default:
		// Add check here? If type is FunctionLiteral and wasn't caught above, it's an error.
		if _, ok := node.(*parser.FunctionLiteral); ok {
			return BadRegister, NewCompileError(node, "compiler internal error: FunctionLiteral fell through switch")
		}
		return BadRegister, NewCompileError(node, fmt.Sprintf("compilation not implemented for %T", node))
	}
	// REMOVED: unreachable return nil // Return nil on success if no specific error occurred in this frame
}

// compileShorthandMethod compiles a shorthand method like methodName() { ... }
// Similar to compileFunctionLiteral but for shorthand method syntax
func (c *Compiler) compileShorthandMethod(node *parser.ShorthandMethod, nameHint string) (uint16, []*Symbol, errors.PaseratiError) {
	// 1. Create a new Compiler instance for the method body
	functionCompiler := newFunctionCompiler(c)

	// 1.1 Mark this as a function body (has implicit 'arguments' binding)
	functionCompiler.isFunctionBody = true

	// 2. Determine and set the function name being compiled
	var determinedFuncName string
	if nameHint != "" {
		determinedFuncName = nameHint
	} else if node.Name != nil {
		determinedFuncName = node.Name.Value
	} else {
		determinedFuncName = "<shorthand-method>"
	}
	functionCompiler.compilingFuncName = determinedFuncName

	// 3. Define inner name in inner scope for recursion
	var funcNameForLookup string
	if node.Name != nil {
		funcNameForLookup = node.Name.Value
		functionCompiler.currentSymbolTable.Define(funcNameForLookup, nilRegister)
	} else if nameHint != "" {
		funcNameForLookup = nameHint
		functionCompiler.currentSymbolTable.Define(funcNameForLookup, nilRegister)
	}

	// 4. Define parameters in the function compiler's enclosed scope
	for _, param := range node.Parameters {
		reg := functionCompiler.regAlloc.Alloc()
		functionCompiler.currentSymbolTable.Define(param.Name.Value, reg)
		// Pin the register since parameters can be captured by inner functions
		functionCompiler.regAlloc.Pin(reg)
	}

	// 5. Handle default parameters
	for _, param := range node.Parameters {
		if param.DefaultValue != nil {
			// Get the parameter's register
			symbol, _, exists := functionCompiler.currentSymbolTable.Resolve(param.Name.Value)
			if !exists {
				// This should not happen if parameter definition worked correctly
				functionCompiler.addError(param.Name, fmt.Sprintf("parameter %s not found in symbol table", param.Name.Value))
				continue
			}
			paramReg := symbol.Register

			// Create a temporary register to hold undefined for comparison
			undefinedReg := functionCompiler.regAlloc.Alloc()
			functionCompiler.emitLoadUndefined(undefinedReg, param.Token.Line)

			// Create another temporary register for the comparison result
			compareReg := functionCompiler.regAlloc.Alloc()
			functionCompiler.emitStrictEqual(compareReg, paramReg, undefinedReg, param.Token.Line)

			// Jump if the comparison is false (parameter is not undefined, so keep original value)
			jumpIfDefinedPos := functionCompiler.emitPlaceholderJump(vm.OpJumpIfFalse, compareReg, param.Token.Line)

			// Free temporary registers
			functionCompiler.regAlloc.Free(undefinedReg)
			functionCompiler.regAlloc.Free(compareReg)

			// Compile the default value expression
			defaultValueReg := functionCompiler.regAlloc.Alloc()
			_, err := functionCompiler.compileNode(param.DefaultValue, defaultValueReg)
			if err != nil {
				// Continue with compilation even if default value has errors
				functionCompiler.addError(param.DefaultValue, fmt.Sprintf("error compiling default value for parameter %s", param.Name.Value))
			} else {
				// Move the default value to the parameter register
				if defaultValueReg != paramReg {
					functionCompiler.emitMove(paramReg, defaultValueReg, param.Token.Line)
				}
			}

			// Patch the jump to come here (end of default value assignment)
			functionCompiler.patchJump(jumpIfDefinedPos)
		}
	}

	// 6. Handle rest parameter (if present)
	if node.RestParameter != nil {
		// Define the rest parameter in the symbol table
		restParamReg := functionCompiler.regAlloc.Alloc()
		// Handle both simple rest parameters (...args) and destructured (...[x, y])
		if node.RestParameter.Name != nil {
			functionCompiler.currentSymbolTable.Define(node.RestParameter.Name.Value, restParamReg)
			debugPrintf("// [Compiler] Rest parameter '%s' defined in R%d\n", node.RestParameter.Name.Value, restParamReg)
		} else if node.RestParameter.Pattern != nil {
			functionCompiler.currentSymbolTable.Define("__rest__", restParamReg)
			debugPrintf("// [Compiler] Rest parameter (destructured) defined in R%d\n", restParamReg)
		}
		// Pin the register since rest parameters can be captured by inner functions
		functionCompiler.regAlloc.Pin(restParamReg)
	}

	// 7. Compile the body using the function compiler
	bodyReg := functionCompiler.regAlloc.Alloc()
	_, err := functionCompiler.compileNode(node.Body, bodyReg)
	functionCompiler.regAlloc.Free(bodyReg) // Free since function body doesn't return a value
	if err != nil {
		// Propagate errors (already appended to c.errors by sub-compiler)
	}

	// 8. Finalize function chunk (add implicit return to the function's chunk)
	functionCompiler.emitFinalReturn(node.Body.Token.Line)
	functionChunk := functionCompiler.chunk
	freeSymbols := functionCompiler.freeSymbols
	// Collect any additional errors from the sub-compilation
	if len(functionCompiler.errors) > 0 {
		c.errors = append(c.errors, functionCompiler.errors...)
	}
	regSize := functionCompiler.regAlloc.MaxRegs()
	functionChunk.NumSpillSlots = int(functionCompiler.nextSpillSlot) // Set spill slots needed

	// 9. Create the bytecode.Function object
	var funcName string
	if nameHint != "" {
		funcName = nameHint
	} else if node.Name != nil {
		funcName = node.Name.Value
	} else {
		funcName = "<shorthand-method>"
	}

	// 10. Add the function object to the outer compiler's constant pool
	// Arity: total parameters excluding 'this' (for VM register allocation)
	// Length: params before first default (for ECMAScript function.length property)
	arity := 0
	length := 0
	seenDefault := false
	for _, param := range node.Parameters {
		if param.IsThis {
			continue
		}
		arity++
		if !seenDefault && param.DefaultValue == nil {
			length++
		} else {
			seenDefault = true
		}
	}
	funcValue := vm.NewFunction(arity, length, len(freeSymbols), int(regSize), node.RestParameter != nil, funcName, functionChunk, false, false, false, functionCompiler.hasLocalCaptures) // isGenerator=false, isAsync=false, isArrowFunction=false
	constIdx := c.chunk.AddConstant(funcValue)

	return constIdx, freeSymbols, nil
}

// compileArgumentsWithOptionalHandling compiles the provided arguments and pads missing optional
// parameters with undefined values. Uses contiguous allocation to place arguments in their final positions.
func (c *Compiler) compileArgumentsWithOptionalHandling(node *parser.CallExpression, firstTargetReg Register) ([]Register, int, errors.PaseratiError) {
	// 1. Determine the expected argument count including optional parameters
	providedArgCount := len(node.Arguments)

	// Get function type to check for optional parameters
	functionType := node.Function.GetComputedType()
	var expectedParamCount int
	var optionalParams []bool

	if functionType != nil {
		if objType, ok := functionType.(*types.ObjectType); ok && objType.IsCallable() && len(objType.CallSignatures) > 0 {
			// TODO: This is a temporary solution. The checker should resolve overloads during type checking
			// and attach the specific selected signature to the call expression.
			// For now, try to pick the best matching signature based on argument count
			sig := objType.CallSignatures[0] // Default to first signature
			bestMatch := sig
			bestScore := -1

			for _, candidateSig := range objType.CallSignatures {
				score := 0
				// Prefer exact parameter count match
				if len(candidateSig.ParameterTypes) == providedArgCount {
					score += 10
				}
				// Or compatible with optional parameters
				requiredParams := 0
				for i, isOptional := range candidateSig.OptionalParams {
					if i < len(candidateSig.ParameterTypes) && !isOptional {
						requiredParams++
					}
				}
				if providedArgCount >= requiredParams && providedArgCount <= len(candidateSig.ParameterTypes) {
					score += 5
				}

				if score > bestScore {
					bestScore = score
					bestMatch = candidateSig
				}
			}

			expectedParamCount = len(bestMatch.ParameterTypes)
			optionalParams = bestMatch.OptionalParams
		}
	}

	// Determine final argument count (provided args + undefined padding for optional params)
	finalArgCount := providedArgCount
	if len(optionalParams) == expectedParamCount && providedArgCount < expectedParamCount {
		// Count how many optional parameters we need to pad
		for i := providedArgCount; i < expectedParamCount; i++ {
			if i < len(optionalParams) && optionalParams[i] {
				finalArgCount++
			} else {
				break // Stop at first required parameter
			}
		}
	}

	// 2. Ensure argument registers exist and build register list
	// CRITICAL: Arguments must be at funcReg+1, funcReg+2, ... (VM calling convention)
	// But these registers may not be allocated yet! We must ensure they're within the function's register space.
	var argRegs []Register
	if finalArgCount > 0 {
		beforeMaxReg := c.regAlloc.maxReg
		// Ensure all argument registers are allocated by calling AllocContiguous
		// This extends maxReg to cover all arguments
		if firstTargetReg == c.regAlloc.nextReg {
			// Perfect case: arguments start right at the allocation frontier
			if debugRegAlloc {
				fmt.Printf("[ARG_ALLOC] Perfect case: firstTarget=R%d == nextReg, allocating %d args\n", firstTargetReg, finalArgCount)
			}
			c.regAlloc.AllocContiguous(finalArgCount)
		} else {
			// Arguments start beyond current allocation - need to ensure they're allocated
			// Calculate how many we need to allocate to reach firstTargetReg + finalArgCount
			// Use int arithmetic to avoid uint8 overflow issues
			lastArgRegInt := int(firstTargetReg) + finalArgCount - 1
			if lastArgRegInt >= int(c.regAlloc.nextReg) {
				// Need to extend allocation to cover these registers
				needed := lastArgRegInt - int(c.regAlloc.nextReg) + 1
				if debugRegAlloc {
					fmt.Printf("[ARG_ALLOC] Gap case: firstTarget=R%d, nextReg=R%d, lastArg=%d, allocating %d more\n",
						firstTargetReg, c.regAlloc.nextReg, lastArgRegInt, needed)
				}
				if needed > 0 {
					c.regAlloc.AllocContiguous(needed)
				}
			} else {
				if debugRegAlloc {
					fmt.Printf("[ARG_ALLOC] Already allocated: firstTarget=R%d, lastArg=%d, nextReg=R%d\n",
						firstTargetReg, lastArgRegInt, c.regAlloc.nextReg)
				}
			}
		}
		afterMaxReg := c.regAlloc.maxReg
		if debugRegAlloc {
			fmt.Printf("[ARG_ALLOC] maxReg: before=%d, after=%d (delta=%d)\n", beforeMaxReg, afterMaxReg, int(afterMaxReg)-int(beforeMaxReg))
		}

		// Now build the argRegs list
		for i := 0; i < finalArgCount; i++ {
			argRegs = append(argRegs, firstTargetReg+Register(i))
		}
	}

	// 3. Compile provided arguments, handling spread syntax
	effectiveArgIndex := 0
	for _, arg := range node.Arguments {
		if spreadElement, isSpread := arg.(*parser.SpreadElement); isSpread {
			// For now, spread syntax should be handled by the new spread call instructions
			// This path should not be reached with proper compiler flow
			return nil, 0, NewCompileError(spreadElement, "spread syntax should be handled by spread call instructions")
		} else {
			// Regular argument - compile directly into its target register
			if effectiveArgIndex >= len(argRegs) {
				return nil, 0, NewCompileError(arg, "too many arguments provided")
			}
			targetReg := argRegs[effectiveArgIndex]
			_, err := c.compileNode(arg, targetReg)
			if err != nil {
				return nil, 0, err
			}
			effectiveArgIndex++
		}
	}

	// 4. Pad missing optional parameters with undefined
	for i := providedArgCount; i < finalArgCount; i++ {
		targetReg := argRegs[i]
		c.emitLoadUndefined(targetReg, node.Token.Line)
	}

	return argRegs, finalArgCount, nil
}

// addFreeSymbol adds a symbol identified as a free variable to the compiler's list.
// It ensures the symbol is added only once and returns its index in the freeSymbols slice.
// Returns a uint16 to support up to 65535 free variables (for large codebases like TSC).
func (c *Compiler) addFreeSymbol(node parser.Node, symbol *Symbol) uint16 {
	debugPrintf("// DEBUG addFreeSymbol: Adding '%s' as free variable (Register: R%d)\n", symbol.Name, symbol.Register)
	for i, free := range c.freeSymbols {
		// Compare by name and register instead of pointer comparison
		// This prevents duplicate upvalues for the same variable
		if free.Name == symbol.Name && free.Register == symbol.Register {
			debugPrintf("// DEBUG addFreeSymbol: Symbol '%s' already exists at index %d (REUSING)\n", symbol.Name, i)
			return uint16(i)
		}
	}
	// Check if we exceed limit (16-bit max for upvalue count in OpClosure16)
	if len(c.freeSymbols) >= 65535 {
		// Handle error: too many free variables
		c.errors = append(c.errors, NewCompileError(node, "compiler: too many free variables in function (max 65535)"))
		return 65535 // Indicate error state
	}
	c.freeSymbols = append(c.freeSymbols, symbol)
	debugPrintf("// DEBUG addFreeSymbol: Added '%s' at index %d (total free symbols: %d)\n", symbol.Name, len(c.freeSymbols)-1, len(c.freeSymbols))
	return uint16(len(c.freeSymbols) - 1)
}

// SetCallerScopeDesc sets the caller's scope descriptor for direct eval compilation.
// This allows eval code to resolve variable names to caller's registers.
// Note: The actual symbol table population happens in Compile() after the symbol table is reset.
// Also sets newTargetAvailable - if scopeDesc is provided, new.target is available from caller.
func (c *Compiler) SetCallerScopeDesc(scopeDesc *vm.ScopeDescriptor) {
	c.callerScopeDesc = scopeDesc
	// For direct eval (scopeDesc != nil), new.target is available from caller's context
	// For indirect eval (scopeDesc == nil), new.target is not available
	c.newTargetAvailable = scopeDesc != nil
}

// GetCallerScopeDesc returns the caller's scope descriptor (if any).
func (c *Compiler) GetCallerScopeDesc() *vm.ScopeDescriptor {
	return c.callerScopeDesc
}

// SetIndirectEval sets the indirect eval mode.
// When true, let/const/class declarations are kept local instead of becoming globals.
// Note: newTargetAvailable is determined by callerScopeDesc - if present (direct eval),
// new.target is available; if absent (indirect eval), new.target is not available.
func (c *Compiler) SetIndirectEval(indirect bool) {
	c.isIndirectEval = indirect
	// newTargetAvailable is determined by SetCallerScopeDesc - don't set it here
}

// IsEvalCreatedBinding checks if a variable name would be a NEW binding created by eval.
// Returns true if we're in direct eval mode AND the name is not from the caller's scope.
// Eval-created bindings should use heap storage to support deletion per ECMAScript spec.
func (c *Compiler) IsEvalCreatedBinding(name string) bool {
	if c.callerScopeDesc == nil {
		// Not in eval mode
		return false
	}
	// Check if the name is in the caller's scope (not deletable)
	for _, localName := range c.callerScopeDesc.LocalNames {
		if localName == name {
			return false // From caller scope, not eval-created
		}
	}
	// New binding created by eval - should be deletable
	return true
}

// ShouldUseHeapForEvalBinding checks if an eval-created binding should use heap storage.
// This enables deletion of eval-created var/function bindings per ECMAScript spec.
// NOTE: This function is only called from let/const declaration paths. Per ECMAScript,
// let/const/class declarations in eval should always be local to the eval's lexical
// environment, never promoted to the outer scope. When isIndirectEval is true (set for
// all eval modes), we return false to keep let/const as local registers.
func (c *Compiler) ShouldUseHeapForEvalBinding(name string) bool {
	// In eval mode, let/const should always be local (not heap)
	if c.isIndirectEval {
		return false
	}
	// Only applies to non-strict direct eval
	if c.callerScopeDesc == nil || c.chunk.IsStrict {
		return false
	}
	return c.IsEvalCreatedBinding(name)
}

// findVarInFunctionScope looks for a var in the current function's scope chain.
// Unlike Resolve(), this stops at function boundaries (doesn't cross into enclosing compilers).
// This is used to check if a var was pre-defined during block hoisting in the SAME function.
// Returns the symbol and the function-level table if found, otherwise returns zero Symbol and nil.
func (c *Compiler) findVarInFunctionScope(name string) (Symbol, *SymbolTable) {
	// Walk up the scope chain within this compiler's function
	funcTable := c.currentSymbolTable
	for funcTable != nil {
		// Check if the name is defined in this scope
		if sym, found := funcTable.store[name]; found {
			return sym, funcTable
		}
		// Stop if we've reached the function-level table (outer is from enclosing compiler or nil)
		if funcTable.Outer == nil {
			break
		}
		if c.enclosing != nil && c.isDefinedInEnclosingCompiler(funcTable.Outer) {
			break
		}
		funcTable = funcTable.Outer
	}
	return Symbol{}, nil
}

// resolveCallerLocal looks up a variable name in the caller's scope descriptor.
// Returns the register index if found, or -1 if not found.
func (c *Compiler) resolveCallerLocal(name string) int {
	if c.callerScopeDesc == nil {
		return -1
	}
	for i, localName := range c.callerScopeDesc.LocalNames {
		if localName == name {
			return i
		}
	}
	return -1
}

// generateScopeDescriptor creates a ScopeDescriptor from the current symbol table.
// This is called when a function contains direct eval, to allow eval code to access local variables.
func (c *Compiler) generateScopeDescriptor() *vm.ScopeDescriptor {
	// Get the maximum register used to size the array
	maxReg := int(c.regAlloc.MaxRegs())
	if maxReg == 0 {
		return &vm.ScopeDescriptor{
			LocalNames:              []string{},
			HasArgumentsBinding:     !c.isArrowFunction, // Non-arrow functions have implicit 'arguments'
			InDefaultParameterScope: c.hasEvalInDefaultParam,
			HasSuperBinding:         c.isMethodCompilation,                  // Methods have [[HomeObject]] for super
			InClassFieldInitializer: c.hasEvalInFieldInitializer,            // Class field initializers forbid 'arguments' in eval
			PrivateBrandStack:       c.convertBrandStackToVM(),              // Private field brand context
			CurrentPrivateBrand:     c.currentPrivateBrand,                  // Current brand ID
			CurrentPrivateBrandInfo: c.convertBrandInfoToVM(&c.currentPrivateBrandInfo),
		}
	}

	// Create array mapping register index to variable name
	localNames := make([]string, maxReg)

	// Use allLocalNames if available (tracks all variables including those from popped scopes)
	// Otherwise fall back to walking the current symbol table
	if len(c.allLocalNames) > 0 {
		for reg, name := range c.allLocalNames {
			if int(reg) < len(localNames) {
				localNames[reg] = name
			}
		}
	} else {
		c.collectLocalNames(c.currentSymbolTable, localNames)
	}

	return &vm.ScopeDescriptor{
		LocalNames:              localNames,
		HasArgumentsBinding:     !c.isArrowFunction, // Non-arrow functions have implicit 'arguments'
		InDefaultParameterScope: c.hasEvalInDefaultParam,
		HasSuperBinding:         c.isMethodCompilation,                  // Methods have [[HomeObject]] for super
		InClassFieldInitializer: c.hasEvalInFieldInitializer,            // Class field initializers forbid 'arguments' in eval
		PrivateBrandStack:       c.convertBrandStackToVM(),              // Private field brand context
		CurrentPrivateBrand:     c.currentPrivateBrand,                  // Current brand ID
		CurrentPrivateBrandInfo: c.convertBrandInfoToVM(&c.currentPrivateBrandInfo),
	}
}

// convertBrandStackToVM converts the compiler's brand stack to VM format for eval.
func (c *Compiler) convertBrandStackToVM() []vm.PrivateBrandInfoVM {
	if len(c.privateBrandStack) == 0 {
		return nil
	}
	result := make([]vm.PrivateBrandInfoVM, len(c.privateBrandStack))
	for i, brandInfo := range c.privateBrandStack {
		converted := c.convertBrandInfoToVM(&brandInfo)
		if converted != nil {
			result[i] = *converted
		} else {
			// Empty brand info - just preserve the brand ID
			result[i] = vm.PrivateBrandInfoVM{BrandID: brandInfo.BrandID}
		}
	}
	return result
}

// convertBrandInfoToVM converts a single PrivateBrandInfo to VM format.
func (c *Compiler) convertBrandInfoToVM(info *PrivateBrandInfo) *vm.PrivateBrandInfoVM {
	if info == nil {
		return nil
	}
	// Handle case where maps might be nil
	var declaredFields map[string]bool
	if info.DeclaredFields != nil {
		declaredFields = make(map[string]bool, len(info.DeclaredFields))
		for k, v := range info.DeclaredFields {
			declaredFields[k] = v
		}
	}
	var memberKinds map[string]vm.PrivateMemberKindVM
	if info.MemberKinds != nil {
		memberKinds = make(map[string]vm.PrivateMemberKindVM, len(info.MemberKinds))
		for k, v := range info.MemberKinds {
			memberKinds[k] = vm.PrivateMemberKindVM(v)
		}
	}
	return &vm.PrivateBrandInfoVM{
		BrandID:        info.BrandID,
		DeclaredFields: declaredFields,
		MemberKinds:    memberKinds,
	}
}

// restoreBrandContextFromVM restores the private brand context from a VM scope descriptor.
// This is used when compiling direct eval code to give it access to enclosing class private fields.
func (c *Compiler) restoreBrandContextFromVM(scopeDesc *vm.ScopeDescriptor) {
	if scopeDesc == nil {
		return
	}

	// Restore brand stack
	if len(scopeDesc.PrivateBrandStack) > 0 {
		c.privateBrandStack = make([]PrivateBrandInfo, len(scopeDesc.PrivateBrandStack))
		for i, vmInfo := range scopeDesc.PrivateBrandStack {
			c.privateBrandStack[i] = c.convertVMBrandInfoToCompiler(&vmInfo)
		}
	}

	// Restore current brand
	c.currentPrivateBrand = scopeDesc.CurrentPrivateBrand
	if scopeDesc.CurrentPrivateBrandInfo != nil {
		c.currentPrivateBrandInfo = c.convertVMBrandInfoToCompiler(scopeDesc.CurrentPrivateBrandInfo)
	}
}

// convertVMBrandInfoToCompiler converts a VM PrivateBrandInfoVM to compiler PrivateBrandInfo.
func (c *Compiler) convertVMBrandInfoToCompiler(vmInfo *vm.PrivateBrandInfoVM) PrivateBrandInfo {
	if vmInfo == nil {
		return PrivateBrandInfo{}
	}
	// Copy declared fields
	declaredFields := make(map[string]bool, len(vmInfo.DeclaredFields))
	for k, v := range vmInfo.DeclaredFields {
		declaredFields[k] = v
	}
	// Convert member kinds
	memberKinds := make(map[string]PrivateMemberKind, len(vmInfo.MemberKinds))
	for k, v := range vmInfo.MemberKinds {
		memberKinds[k] = PrivateMemberKind(v)
	}
	return PrivateBrandInfo{
		BrandID:        vmInfo.BrandID,
		DeclaredFields: declaredFields,
		MemberKinds:    memberKinds,
	}
}

// collectLocalNames recursively collects local variable names from the symbol table chain
func (c *Compiler) collectLocalNames(st *SymbolTable, localNames []string) {
	if st == nil {
		return
	}

	// Collect from current scope
	for name, symbol := range st.store {
		if !symbol.IsGlobal && int(symbol.Register) < len(localNames) {
			localNames[symbol.Register] = name
		}
	}

	// Note: We don't recurse into outer scopes here because:
	// 1. For function compilers, currentSymbolTable is the function's own scope
	// 2. Outer scopes belong to enclosing functions (handled by upvalues, not direct access)
}

// trackLocalName records a local variable allocation in allLocalNames for ScopeDescriptor generation.
// This must be called whenever a local (non-global) variable is defined with a register.
func (c *Compiler) trackLocalName(name string, reg Register) {
	if c.allLocalNames != nil && reg != nilRegister {
		c.allLocalNames[reg] = name
	}
}

// emitPlaceholderJump emits a jump instruction with a placeholder offset (0xFFFF).
// Returns the position of the start of the jump instruction in the bytecode.
// For OpJumpIfFalse, srcReg is the condition register.
// For OpJump, srcReg is ignored (pass 0 or any value).
func (c *Compiler) emitPlaceholderJump(op vm.OpCode, srcReg Register, line int) int {
	pos := len(c.chunk.Code)
	if debugCompiler {
		fmt.Printf("[EMIT PLACEHOLDER] pos=%d op=%v reg=%d line=%d func=%s\n",
			pos, op, srcReg, line, c.compilingFuncName)
	}
	c.emitOpCode(op, line)
	if op == vm.OpJumpIfFalse || op == vm.OpJumpIfUndefined || op == vm.OpJumpIfNull || op == vm.OpJumpIfNullish {
		c.emitByte(byte(srcReg)) // Register operand
		c.emitUint16(0xFFFF)     // Placeholder offset
	} else { // OpJump
		c.emitUint16(0xFFFF) // Placeholder offset
	}
	return pos
}

// patchJump calculates the distance from a placeholder jump instruction
// to the current end of the bytecode and writes the offset back.
func (c *Compiler) patchJump(placeholderPos int) {
	op := vm.OpCode(c.chunk.Code[placeholderPos])
	operandStartPos := placeholderPos + 1
	if op == vm.OpJumpIfFalse || op == vm.OpJumpIfUndefined || op == vm.OpJumpIfNull || op == vm.OpJumpIfNullish {
		operandStartPos = placeholderPos + 2 // Skip register byte
	}
	// OpPushBreak and OpPushContinue have no register operand, just the offset

	// Calculate offset from the position *after* the jump instruction
	jumpInstructionEndPos := operandStartPos + 2
	offset := len(c.chunk.Code) - jumpInstructionEndPos

	if debugCompiler {
		fmt.Printf("[PATCH JUMP] pos=%d op=%v offset=%d (from %d to %d)\n",
			placeholderPos, op, offset, jumpInstructionEndPos, len(c.chunk.Code))
	}

	if offset > math.MaxInt16 || offset < math.MinInt16 { // Use math constants
		// Handle error: jump offset too large
		// TODO: Add proper error handling instead of panic
		panic(fmt.Sprintf("Compiler error: jump offset %d exceeds 16-bit limit", offset))
	}

	// Write the 16-bit offset back into the placeholder bytes (Big Endian)
	c.chunk.Code[operandStartPos] = byte(int16(offset) >> 8)     // High byte
	c.chunk.Code[operandStartPos+1] = byte(int16(offset) & 0xFF) // Low byte
}

// patchJumpToTarget patches a jump to a specific target PC
func (c *Compiler) patchJumpToTarget(placeholderPos int, targetPC int) {
	op := vm.OpCode(c.chunk.Code[placeholderPos])
	operandStartPos := placeholderPos + 1
	if op == vm.OpJumpIfFalse || op == vm.OpJumpIfUndefined || op == vm.OpJumpIfNull || op == vm.OpJumpIfNullish {
		operandStartPos = placeholderPos + 2 // Skip register byte
	}

	// Calculate offset from the position *after* the jump instruction to the target
	jumpInstructionEndPos := operandStartPos + 2
	offset := targetPC - jumpInstructionEndPos

	if debugCompiler {
		fmt.Printf("[PATCH JUMP TO TARGET] pos=%d op=%v offset=%d (from %d to %d)\n",
			placeholderPos, op, offset, jumpInstructionEndPos, targetPC)
	}

	if offset > math.MaxInt16 || offset < math.MinInt16 {
		panic(fmt.Sprintf("Compiler error: jump offset %d exceeds 16-bit limit", offset))
	}

	// Write the 16-bit offset back into the placeholder bytes (Big Endian)
	c.chunk.Code[operandStartPos] = byte(int16(offset) >> 8)     // High byte
	c.chunk.Code[operandStartPos+1] = byte(int16(offset) & 0xFF) // Low byte
}

// storeToLvalue is a helper function to store a value back to different types of lvalues
func (c *Compiler) storeToLvalue(lvalueKind int, identInfo, memberInfo, indexInfo interface{}, valueReg Register, line int) {
	const (
		lvalueIdentifier = iota
		lvalueMemberExpr
		lvalueIndexExpr
	)

	switch lvalueKind {
	case lvalueIdentifier:
		info := identInfo.(struct {
			targetReg    Register
			isUpvalue    bool
			upvalueIndex uint16 // 16-bit to support large closures
			isGlobal     bool
			globalIndex  uint16
		})
		if info.isGlobal {
			c.emitSetGlobal(info.globalIndex, valueReg, line)
		} else if info.isUpvalue {
			c.emitSetUpvalue(info.upvalueIndex, valueReg, line)
		} else {
			if valueReg != info.targetReg {
				c.emitMove(info.targetReg, valueReg, line)
			}
		}

	case lvalueMemberExpr:
		info := memberInfo.(struct {
			objectReg      Register
			nameConstIdx   uint16
			isPrivateField bool
		})
		if info.isPrivateField {
			c.emitSetPrivateField(info.objectReg, valueReg, info.nameConstIdx, line)
		} else {
			c.emitSetProp(info.objectReg, valueReg, info.nameConstIdx, line)
		}

	case lvalueIndexExpr:
		info := indexInfo.(struct {
			arrayReg Register
			indexReg Register
		})
		c.emitOpCode(vm.OpSetIndex, line)
		c.emitByte(byte(info.arrayReg))
		c.emitByte(byte(info.indexReg))
		c.emitByte(byte(valueReg))
	}
}

// addError creates a CompileError and appends it to the compiler's error list.
func (c *Compiler) addError(node parser.Node, msg string) {
	token := parser.GetTokenFromNode(node)
	compileErr := &errors.CompileError{
		Position: errors.Position{
			Line:     token.Line,
			Column:   token.Column,
			StartPos: token.StartPos,
			EndPos:   token.EndPos,
		},
		Msg: msg,
	}
	c.errors = append(c.errors, compileErr)
}

func NewCompileError(node parser.Node, msg string) *errors.CompileError {
	token := parser.GetTokenFromNode(node)
	return &errors.CompileError{
		Position: errors.Position{
			Line:     token.Line,
			Column:   token.Column,
			StartPos: token.StartPos,
			EndPos:   token.EndPos,
		},
		Msg: msg,
	}
}

// pushLoopContext adds a new loop context to the stack.
func (c *Compiler) pushLoopContext(loopStartPos, continueTargetPos int) {
	context := &LoopContext{
		LoopStartPos:               loopStartPos,
		ContinueTargetPos:          continueTargetPos,
		BreakPlaceholderPosList:    make([]int, 0),
		ContinuePlaceholderPosList: make([]int, 0),
	}
	c.loopContextStack = append(c.loopContextStack, context)
}

// popLoopContext removes the current loop context from the stack.
func (c *Compiler) popLoopContext() {
	if len(c.loopContextStack) > 0 {
		c.loopContextStack = c.loopContextStack[:len(c.loopContextStack)-1]
	}
	// Consider adding error handling if pop is called on an empty stack
}

// currentLoopContext returns the loop context currently at the top of the stack, or nil if empty.
func (c *Compiler) currentLoopContext() *LoopContext {
	if len(c.loopContextStack) == 0 {
		return nil
	}
	return c.loopContextStack[len(c.loopContextStack)-1]
}

// --- Private Field Brand Tracking ---
// Each class gets a unique brand ID to distinguish private fields with the same name
// across different classes (Parent.#field vs Child.#field should be different)
// Additionally, we track which private field names each class declares, so nested classes
// can access outer class private fields using the outer class's brand.

// PrivateMemberKind indicates what kind of private member is declared
type PrivateMemberKind int

const (
	PrivateMemberField    PrivateMemberKind = iota // Regular data field
	PrivateMemberMethod                            // Private method
	PrivateMemberGetter                            // Private getter
	PrivateMemberSetter                            // Private setter
	PrivateMemberAccessor                          // Private getter+setter
)

// PrivateBrandInfo tracks a class's brand ID and the private field names it declares
type PrivateBrandInfo struct {
	BrandID        int
	DeclaredFields map[string]bool              // Set of private field names (without #) this class declares
	MemberKinds    map[string]PrivateMemberKind // Kind of each private member
}

// enterClassBrand assigns a new unique brand ID and pushes it onto the stack.
// Call this when entering a class body compilation.
func (c *Compiler) enterClassBrand() int {
	// Get brand counter from top-level compiler
	topLevel := c.getTopLevelCompiler()
	topLevel.privateBrandCounter++
	brandID := topLevel.privateBrandCounter

	// Push current brand info onto stack before changing
	c.privateBrandStack = append(c.privateBrandStack, c.currentPrivateBrandInfo)
	c.currentPrivateBrandInfo = PrivateBrandInfo{
		BrandID:        brandID,
		DeclaredFields: make(map[string]bool),
		MemberKinds:    make(map[string]PrivateMemberKind),
	}
	c.currentPrivateBrand = brandID

	debugPrintf("// DEBUG enterClassBrand: Assigned brand ID %d\n", brandID)
	return brandID
}

// exitClassBrand pops the current brand and restores the previous one.
// Call this when exiting a class body compilation.
func (c *Compiler) exitClassBrand() {
	if len(c.privateBrandStack) > 0 {
		c.currentPrivateBrandInfo = c.privateBrandStack[len(c.privateBrandStack)-1]
		c.privateBrandStack = c.privateBrandStack[:len(c.privateBrandStack)-1]
		c.currentPrivateBrand = c.currentPrivateBrandInfo.BrandID
	} else {
		c.currentPrivateBrandInfo = PrivateBrandInfo{}
		c.currentPrivateBrand = 0
	}
	debugPrintf("// DEBUG exitClassBrand: Restored brand ID %d\n", c.currentPrivateBrand)
}

// declarePrivateField registers a private field name as declared by the current class.
// Call this when compiling private field/method/accessor declarations.
func (c *Compiler) declarePrivateField(fieldName string) {
	c.declarePrivateMember(fieldName, PrivateMemberField)
}

// declarePrivateMember registers a private member with its kind.
func (c *Compiler) declarePrivateMember(fieldName string, kind PrivateMemberKind) {
	if c.currentPrivateBrandInfo.DeclaredFields != nil {
		c.currentPrivateBrandInfo.DeclaredFields[fieldName] = true
		// For accessors, merge getter+setter into accessor kind
		if existingKind, exists := c.currentPrivateBrandInfo.MemberKinds[fieldName]; exists {
			if (existingKind == PrivateMemberGetter && kind == PrivateMemberSetter) ||
				(existingKind == PrivateMemberSetter && kind == PrivateMemberGetter) {
				c.currentPrivateBrandInfo.MemberKinds[fieldName] = PrivateMemberAccessor
			}
		} else {
			c.currentPrivateBrandInfo.MemberKinds[fieldName] = kind
		}
		debugPrintf("// DEBUG declarePrivateMember: Declared '%s' (kind=%d) for brand %d\n", fieldName, kind, c.currentPrivateBrand)
	}
}

// getPrivateMemberKind returns the kind of a private member declared in the current scope chain.
// Returns (kind, brandID, found). If found is false, the member wasn't declared.
func (c *Compiler) getPrivateMemberKind(fieldName string) (PrivateMemberKind, int, bool) {
	// First check current class
	if c.currentPrivateBrandInfo.MemberKinds != nil {
		if kind, exists := c.currentPrivateBrandInfo.MemberKinds[fieldName]; exists {
			return kind, c.currentPrivateBrand, true
		}
	}

	// Walk the stack from most recent to oldest
	for i := len(c.privateBrandStack) - 1; i >= 0; i-- {
		brandInfo := c.privateBrandStack[i]
		if brandInfo.MemberKinds != nil {
			if kind, exists := brandInfo.MemberKinds[fieldName]; exists {
				return kind, brandInfo.BrandID, true
			}
		}
	}

	return PrivateMemberField, 0, false
}

// getPrivateFieldKey returns the branded key for a private field.
// It looks up which class in the current scope chain declares this field.
// Format: "brandID:fieldName" (e.g., "1:field" for brand 1 and field "field")
func (c *Compiler) getPrivateFieldKey(fieldName string) string {
	// First check if current class declares this field
	if c.currentPrivateBrandInfo.DeclaredFields != nil && c.currentPrivateBrandInfo.DeclaredFields[fieldName] {
		return fmt.Sprintf("%d:%s", c.currentPrivateBrand, fieldName)
	}

	// Walk the stack from most recent to oldest to find the declaring class
	for i := len(c.privateBrandStack) - 1; i >= 0; i-- {
		brandInfo := c.privateBrandStack[i]
		if brandInfo.DeclaredFields != nil && brandInfo.DeclaredFields[fieldName] {
			return fmt.Sprintf("%d:%s", brandInfo.BrandID, fieldName)
		}
	}

	// Not found in any class - use current brand (fallback)
	if c.currentPrivateBrand == 0 {
		return fieldName
	}
	return fmt.Sprintf("%d:%s", c.currentPrivateBrand, fieldName)
}

// getTopLevelCompiler returns the outermost compiler (for accessing global counters)
func (c *Compiler) getTopLevelCompiler() *Compiler {
	top := c
	for top.enclosing != nil {
		top = top.enclosing
	}
	return top
}

// --- NEW: Bytecode Position Helper ---

// currentPosition returns the index of the next byte to be written to the chunk.
func (c *Compiler) currentPosition() int {
	return len(c.chunk.Code)
}

// emitClosure emits the OpClosure instruction and its operands.
// It handles resolving free variables from the *enclosing* scope (c)
// based on the freeSymbols list collected during the function body's compilation.
// NOTE: We always emit OpClosure even with 0 upvalues to ensure each call creates
// a fresh closure with its own Properties (for .prototype etc). Using OpLoadConstant
// would share the FunctionObject, causing .prototype to be shared across calls.
func (c *Compiler) emitClosure(destReg Register, funcConstIndex uint16, node *parser.FunctionLiteral, freeSymbols []*Symbol) Register {
	line := node.Token.Line // Use function literal token line

	// Determine the name used for potential self-recursion lookup within the function body.
	var funcNameForLookup string
	if node.Name != nil {
		funcNameForLookup = node.Name.Value
	}

	// PHASE 1: Collect upvalue descriptors for all free symbols.
	type upvalueInfo struct {
		captureType UpvalueCaptureType
		index       uint16 // Can be register (0-255), upvalue index (0-255), or spill index (0-65535)
	}
	upvalueDescriptors := make([]upvalueInfo, len(freeSymbols))

	for i, freeSym := range freeSymbols {
		debugPrintf("// [emitClosure %s] Processing upvalue %d: %s (Original Reg: R%d)\n", funcNameForLookup, i, freeSym.Name, freeSym.Register)

		// Check for self-capture
		if freeSym.Register == nilRegister && funcNameForLookup != "" && freeSym.Name == funcNameForLookup {
			debugPrintf("// [emitClosure SelfCapture] Symbol '%s' is self-reference, will capture from destReg=R%d\n", freeSym.Name, destReg)
			upvalueDescriptors[i] = upvalueInfo{captureType: CaptureFromRegister, index: uint16(destReg)}
			continue
		}

		// Resolve the symbol in the enclosing compiler's context
		enclosingSymbol, enclosingTable, found := c.currentSymbolTable.Resolve(freeSym.Name)
		if !found {
			panic(fmt.Sprintf("compiler internal error: free variable '%s' not found in enclosing scope during closure emission", freeSym.Name))
		}

		// Check if the variable is in the current compiler's scope chain (local capture)
		// vs in an enclosing compiler's scope chain (upvalue capture)
		isInCurrentScope := c.isInCurrentScopeChain(enclosingTable)
		debugPrintf("// [emitClosure] Checking '%s': enclosingTable=%p, currentTable=%p, c.compilingFuncName=%s, c.enclosing=%v, isInCurrentScope=%v, IsSpilled=%v\n",
			freeSym.Name, enclosingTable, c.currentSymbolTable, c.compilingFuncName, c.enclosing != nil, isInCurrentScope, enclosingSymbol.IsSpilled)

		if isInCurrentScope {
			// Variable is local in the current function - mark that our locals are captured
			c.hasLocalCaptures = true
			if enclosingSymbol.IsSpilled {
				// Spilled variable: capture directly from spill slot
				// The VM will create a closed upvalue with the spilled value
				debugPrintf("// [emitClosure] Free '%s' is SPILLED (slot %d), will capture from spill slot\n", freeSym.Name, enclosingSymbol.SpillIndex)
				if enclosingSymbol.SpillIndex <= 255 {
					upvalueDescriptors[i] = upvalueInfo{captureType: CaptureFromSpill, index: enclosingSymbol.SpillIndex}
				} else {
					upvalueDescriptors[i] = upvalueInfo{captureType: CaptureFromSpill16, index: enclosingSymbol.SpillIndex}
				}
			} else {
				debugPrintf("// [emitClosure] Free '%s' is Local in same function, will capture from R%d\n", freeSym.Name, enclosingSymbol.Register)
				c.regAlloc.Pin(enclosingSymbol.Register)
				upvalueDescriptors[i] = upvalueInfo{captureType: CaptureFromRegister, index: uint16(enclosingSymbol.Register)}
			}
		} else {
			// Variable is from an outer function's scope
			enclosingFreeIndex := c.addFreeSymbol(node, &enclosingSymbol)
			debugPrintf("// [emitClosure] Free '%s' is from Outer function, upvalue index=%d\n", freeSym.Name, enclosingFreeIndex)
			if enclosingFreeIndex > 255 {
				upvalueDescriptors[i] = upvalueInfo{captureType: CaptureFromUpvalue16, index: enclosingFreeIndex}
			} else {
				upvalueDescriptors[i] = upvalueInfo{captureType: CaptureFromUpvalue, index: enclosingFreeIndex}
			}
		}
	}

	// PHASE 2: Now emit OpClosure (or OpClosure16 for >255 upvalues) with all upvalue descriptors
	upvalueCount := len(freeSymbols)
	if upvalueCount > 255 {
		// Use OpClosure16 for large closures (16-bit upvalue count)
		c.emitOpCode(vm.OpClosure16, line)
		c.emitByte(byte(destReg))
		c.emitUint16(funcConstIndex)
		c.emitByte(byte(upvalueCount >> 8))   // High byte of upvalue count
		c.emitByte(byte(upvalueCount & 0xFF)) // Low byte of upvalue count
	} else {
		// Use standard OpClosure (8-bit upvalue count)
		c.emitOpCode(vm.OpClosure, line)
		c.emitByte(byte(destReg))
		c.emitUint16(funcConstIndex)
		c.emitByte(byte(upvalueCount))
	}

	// Emit all upvalue descriptors
	for i, desc := range upvalueDescriptors {
		debugPrintf("// [emitClosure] Emitting upvalue %d: captureType=%d, index=%d\n", i, desc.captureType, desc.index)
		c.emitByte(byte(desc.captureType))
		if desc.captureType == CaptureFromSpill16 || desc.captureType == CaptureFromUpvalue16 {
			// 16-bit index: emit high byte then low byte
			c.emitByte(byte(desc.index >> 8))
			c.emitByte(byte(desc.index & 0xFF))
		} else {
			// 8-bit index for registers, upvalues, and small spill indices
			c.emitByte(byte(desc.index))
		}
	}

	debugPrintf("// [emitClosure %s] Closure emitted to R%d. Set lastExprReg/Valid.\n", funcNameForLookup, destReg)
	return destReg
}

// emitClosureGeneric is a generic version of emitClosure that works with any node type
// that has Token.Line and Name fields (like ShorthandMethod)
// OPTIMIZATION: If there are no upvalues, just load the function constant directly.
func (c *Compiler) emitClosureGeneric(destReg Register, funcConstIndex uint16, line int, nameNode *parser.Identifier, freeSymbols []*Symbol) Register {
	// OPTIMIZATION: If no upvalues, just load the function constant
	if len(freeSymbols) == 0 {
		debugPrintf("// [emitClosureGeneric OPTIMIZED] No upvalues, using OpLoadConstant instead of OpClosure\n")
		c.emitLoadConstant(destReg, funcConstIndex, line)
		return destReg
	}

	// Determine the name used for potential self-recursion lookup
	var funcNameForLookup string
	if nameNode != nil {
		funcNameForLookup = nameNode.Value
	}

	// PHASE 1: Collect upvalue descriptors for all free symbols.
	type upvalueInfo struct {
		captureType UpvalueCaptureType
		index       uint16 // Can be register (0-255), upvalue index (0-255), or spill index (0-65535)
	}
	upvalueDescriptors := make([]upvalueInfo, len(freeSymbols))

	for i, freeSym := range freeSymbols {
		debugPrintf("// [emitClosureGeneric %s] Processing upvalue %d: %s (Original Reg: R%d)\n", funcNameForLookup, i, freeSym.Name, freeSym.Register)

		// Check for self-capture
		if freeSym.Register == nilRegister && funcNameForLookup != "" && freeSym.Name == funcNameForLookup {
			debugPrintf("// [emitClosureGeneric SelfCapture] Symbol '%s' is self-reference, will capture from destReg=R%d\n", freeSym.Name, destReg)
			upvalueDescriptors[i] = upvalueInfo{captureType: CaptureFromRegister, index: uint16(destReg)}
			continue
		}

		// Resolve the symbol in the enclosing compiler's context
		enclosingSymbol, enclosingTable, found := c.currentSymbolTable.Resolve(freeSym.Name)
		if !found {
			panic(fmt.Sprintf("compiler internal error: free variable '%s' not found in enclosing scope during closure emission", freeSym.Name))
		}

		// Check if the variable is in the current compiler's scope chain (local capture)
		// vs in an enclosing compiler's scope chain (upvalue capture)
		isInCurrentScope := c.isInCurrentScopeChain(enclosingTable)

		if isInCurrentScope {
			// Variable is local in the current function - mark that our locals are captured
			c.hasLocalCaptures = true
			if enclosingSymbol.IsSpilled {
				// Spilled variable: capture directly from spill slot
				debugPrintf("// [emitClosureGeneric] Free '%s' is SPILLED (slot %d), will capture from spill slot\n", freeSym.Name, enclosingSymbol.SpillIndex)
				if enclosingSymbol.SpillIndex <= 255 {
					upvalueDescriptors[i] = upvalueInfo{captureType: CaptureFromSpill, index: enclosingSymbol.SpillIndex}
				} else {
					upvalueDescriptors[i] = upvalueInfo{captureType: CaptureFromSpill16, index: enclosingSymbol.SpillIndex}
				}
			} else {
				debugPrintf("// [emitClosureGeneric] Free '%s' is Local in same function, will capture from R%d\n", freeSym.Name, enclosingSymbol.Register)
				c.regAlloc.Pin(enclosingSymbol.Register)
				upvalueDescriptors[i] = upvalueInfo{captureType: CaptureFromRegister, index: uint16(enclosingSymbol.Register)}
			}
		} else {
			// Variable is from an outer function's scope
			dummyNode := &parser.Identifier{Token: lexer.Token{}, Value: freeSym.Name}
			enclosingFreeIndex := c.addFreeSymbol(dummyNode, &enclosingSymbol)
			debugPrintf("// [emitClosureGeneric] Free '%s' is from Outer function, upvalue index=%d\n", freeSym.Name, enclosingFreeIndex)
			if enclosingFreeIndex > 255 {
				upvalueDescriptors[i] = upvalueInfo{captureType: CaptureFromUpvalue16, index: enclosingFreeIndex}
			} else {
				upvalueDescriptors[i] = upvalueInfo{captureType: CaptureFromUpvalue, index: enclosingFreeIndex}
			}
		}
	}

	// PHASE 2: Now emit OpClosure (or OpClosure16 for >255 upvalues) with all upvalue descriptors
	upvalueCount := len(freeSymbols)
	if upvalueCount > 255 {
		// Use OpClosure16 for large closures (16-bit upvalue count)
		c.emitOpCode(vm.OpClosure16, line)
		c.emitByte(byte(destReg))
		c.emitUint16(funcConstIndex)
		c.emitByte(byte(upvalueCount >> 8))   // High byte of upvalue count
		c.emitByte(byte(upvalueCount & 0xFF)) // Low byte of upvalue count
	} else {
		// Use standard OpClosure (8-bit upvalue count)
		c.emitOpCode(vm.OpClosure, line)
		c.emitByte(byte(destReg))
		c.emitUint16(funcConstIndex)
		c.emitByte(byte(upvalueCount))
	}

	// Emit all upvalue descriptors
	for i, desc := range upvalueDescriptors {
		debugPrintf("// [emitClosureGeneric] Emitting upvalue %d: captureType=%d, index=%d\n", i, desc.captureType, desc.index)
		c.emitByte(byte(desc.captureType))
		if desc.captureType == CaptureFromSpill16 || desc.captureType == CaptureFromUpvalue16 {
			// 16-bit index: emit high byte then low byte
			c.emitByte(byte(desc.index >> 8))
			c.emitByte(byte(desc.index & 0xFF))
		} else {
			// 8-bit index for registers, upvalues, and small spill indices
			c.emitByte(byte(desc.index))
		}
	}

	debugPrintf("// [emitClosureGeneric %s] Closure emitted to R%d. Set lastExprReg/Valid.\n", funcNameForLookup, destReg)
	return destReg
}

// resolveRegisterMoves handles moving values from sourceRegs to consecutive target registers
// starting at firstTargetReg. It correctly handles register cycles by using temporary registers.
func (c *Compiler) resolveRegisterMoves(sourceRegs []Register, firstTargetReg Register, line int) {
	argCount := len(sourceRegs)
	if argCount == 0 {
		return
	}

	// Create mapping of source -> target
	moves := make(map[Register]Register)
	for i, sourceReg := range sourceRegs {
		targetReg := firstTargetReg + Register(i)
		if sourceReg != targetReg {
			moves[sourceReg] = targetReg
		}
	}

	debugPrintf("// DEBUG RegisterMoves: Processing %d moves\n", len(moves))
	for source, target := range moves {
		debugPrintf("// DEBUG RegisterMoves: R%d -> R%d\n", source, target)
	}

	// Track which registers have been resolved
	resolved := make(map[Register]bool)

	// Process each move, handling cycles
	for sourceReg, targetReg := range moves {
		if resolved[sourceReg] {
			continue
		}

		// Check if this is part of a cycle
		cycle := c.findMoveCycle(sourceReg, moves, resolved)

		debugPrintf("// DEBUG RegisterMoves: Found cycle starting from R%d: %v\n", sourceReg, cycle)

		if len(cycle) > 1 {
			// Handle cycle by using a temporary register
			debugPrintf("// DEBUG RegisterMoves: Resolving cycle of length %d\n", len(cycle))
			c.resolveCycle(cycle, moves, line)
			// Mark all registers in the cycle as resolved
			for _, reg := range cycle {
				resolved[reg] = true
			}
		} else {
			// Simple move, no cycle
			debugPrintf("// DEBUG RegisterMoves: Simple move R%d -> R%d\n", sourceReg, targetReg)
			c.emitMove(targetReg, sourceReg, line)
			resolved[sourceReg] = true
		}
	}
}

// findMoveCycle detects if a register is part of a move cycle
func (c *Compiler) findMoveCycle(startReg Register, moves map[Register]Register, resolved map[Register]bool) []Register {
	visited := make(map[Register]bool)
	path := []Register{}
	current := startReg

	for {
		if resolved[current] {
			// Already resolved, no cycle involving this register
			return []Register{startReg}
		}

		if visited[current] {
			// Found a cycle - find where it starts
			cycleStart := -1
			for i, reg := range path {
				if reg == current {
					cycleStart = i
					break
				}
			}
			if cycleStart >= 0 {
				return path[cycleStart:] // Return just the cycle portion
			}
			break
		}

		visited[current] = true
		path = append(path, current)

		// Follow the move chain
		next, exists := moves[current]
		if !exists {
			// Chain ends here, no cycle
			return []Register{startReg}
		}
		current = next
	}

	// No cycle found
	return []Register{startReg}
}

// resolveCycle breaks a register move cycle using a temporary register
func (c *Compiler) resolveCycle(cycle []Register, moves map[Register]Register, line int) {
	if len(cycle) <= 1 {
		return
	}

	debugPrintf("// DEBUG ResolveCycle: Cycle %v\n", cycle)

	// Use a temporary register to break the cycle
	tempReg := c.regAlloc.Alloc()
	debugPrintf("// DEBUG ResolveCycle: Using temp register R%d\n", tempReg)

	// Move the first register to temp
	debugPrintf("// DEBUG ResolveCycle: Move R%d -> R%d (first to temp)\n", cycle[0], tempReg)
	c.emitMove(tempReg, cycle[0], line)

	// Move each register to the next in sequence
	for i := 0; i < len(cycle)-1; i++ {
		sourceReg := cycle[i+1]
		targetReg := moves[cycle[i]] // Fix: use moves[cycle[i]], not moves[cycle[i+1]]
		debugPrintf("// DEBUG ResolveCycle: Move R%d -> R%d (chain move %d)\n", sourceReg, targetReg, i)
		c.emitMove(targetReg, sourceReg, line)
	}

	// Move temp to the final position
	firstReg := cycle[0]
	targetReg := moves[firstReg]
	debugPrintf("// DEBUG ResolveCycle: Move R%d -> R%d (temp to final)\n", tempReg, targetReg)
	c.emitMove(targetReg, tempReg, line)

	// Free the temporary register
	c.regAlloc.Free(tempReg)
	debugPrintf("// DEBUG ResolveCycle: Freed temp register R%d\n", tempReg)
}

// --- NEW: Global Variable Index Management ---

// GetOrAssignGlobalIndex returns the index for a global variable name.
// If the variable doesn't have an index yet, assigns a new one.
// Uses the unified heap allocator if available, otherwise falls back to legacy system.
func (c *Compiler) GetOrAssignGlobalIndex(name string) uint16 {
	// Only top-level compiler should manage global indices
	topCompiler := c
	for topCompiler.enclosing != nil {
		topCompiler = topCompiler.enclosing
	}

	// Use heap allocator if available (new unified system)
	if topCompiler.heapAlloc != nil {
		idx := topCompiler.heapAlloc.GetOrAssignIndex(name)
		if idx > 65535 {
			panic(fmt.Sprintf("Too many global variables (max 65536): %s", name))
		}
		return uint16(idx)
	}

	// Fall back to legacy system
	if idx, exists := topCompiler.globalIndices[name]; exists {
		// debugPrintf("// [Compiler] GetOrAssignGlobalIndex: '%s' already has index %d\n", name, idx)
		return uint16(idx)
	}

	// Assign new index
	idx := topCompiler.globalCount
	topCompiler.globalIndices[name] = idx
	topCompiler.globalCount++
	// debugPrintf("// [Compiler] GetOrAssignGlobalIndex: Assigned new index %d to '%s' (total globals: %d)\n", idx, name, topCompiler.globalCount)

	if idx > 65535 {
		panic(fmt.Sprintf("Too many global variables (max 65536): %s", name))
	}

	return uint16(idx)
}

// GlobalExists checks if a global variable with the given name has already been registered.
// This is used to distinguish between existing globals and truly undeclared variables.
func (c *Compiler) GlobalExists(name string) bool {
	// Only top-level compiler manages global indices
	topCompiler := c
	for topCompiler.enclosing != nil {
		topCompiler = topCompiler.enclosing
	}

	// Use heap allocator if available (new unified system)
	if topCompiler.heapAlloc != nil {
		_, exists := topCompiler.heapAlloc.GetIndex(name)
		return exists
	}

	// Fall back to legacy system
	_, exists := topCompiler.globalIndices[name]
	return exists
}

// MarkVarGlobal registers a global index as a var declaration (non-configurable per ECMAScript)
// This should be called for top-level var declarations, not for let/const or eval-created bindings.
func (c *Compiler) MarkVarGlobal(idx uint16) {
	// Only top-level compiler should manage the chunk's var global list
	topCompiler := c
	for topCompiler.enclosing != nil {
		topCompiler = topCompiler.enclosing
	}
	topCompiler.chunk.AddVarGlobalIndex(idx)
}

// GetGlobalNames returns a slice of all global variable names that have been assigned indices
func (c *Compiler) GetGlobalNames() []string {
	// Only top-level compiler should manage global indices
	topCompiler := c
	for topCompiler.enclosing != nil {
		topCompiler = topCompiler.enclosing
	}

	names := make([]string, 0, len(topCompiler.globalIndices))
	for name := range topCompiler.globalIndices {
		names = append(names, name)
	}
	return names
}

// GetGlobalIndex returns the index for a global variable name, or -1 if not found
func (c *Compiler) GetGlobalIndex(name string) int {
	// Only top-level compiler should manage global indices
	topCompiler := c
	for topCompiler.enclosing != nil {
		topCompiler = topCompiler.enclosing
	}

	// Use heap allocator if available (new unified system)
	if topCompiler.heapAlloc != nil {
		if idx, exists := topCompiler.heapAlloc.GetIndex(name); exists {
			return idx
		}
		return -1
	}

	// Fall back to legacy system
	if idx, exists := topCompiler.globalIndices[name]; exists {
		return idx
	}
	return -1
}

// SetGlobalIndex forces a global variable name to use a specific index
// This is used to coordinate global indices between different compiler instances
func (c *Compiler) SetGlobalIndex(name string, index int) {
	// Only top-level compiler should manage global indices
	topCompiler := c
	for topCompiler.enclosing != nil {
		topCompiler = topCompiler.enclosing
	}

	topCompiler.globalIndices[name] = index
	// Update global count if this index is beyond current count
	if index >= topCompiler.globalCount {
		topCompiler.globalCount = index + 1
	}
	// debugPrintf("// [Compiler] SetGlobalIndex: Forced '%s' to index %d\n", name, index)
}

// GetExportGlobalIndices returns a mapping from export names to their global indices
// This allows the VM to efficiently collect export values from the global table
func (c *Compiler) GetExportGlobalIndices() map[string]int {
	if !c.IsModuleMode() {
		return make(map[string]int)
	}

	exportIndices := make(map[string]int)

	// Get all exported names from module bindings
	for exportName, exportRef := range c.moduleBindings.ExportedNames {
		if exportRef.IsReExport {
			// For re-exports, we need to look up the import that corresponds to this re-export
			// Re-exports are handled by import declarations, so find the corresponding import
			if importRef, exists := c.moduleBindings.ImportedNames[exportRef.LocalName]; exists {
				if globalIdx := importRef.GlobalIndex; globalIdx >= 0 {
					exportIndices[exportName] = globalIdx
					// fmt.Printf("// [Compiler] GetExportGlobalIndices: Re-export '%s' maps to imported global[%d]\n", exportName, globalIdx)
				}
			} else {
				// Re-export might be an unnamed re-export (export * from), try to find by local name
				if globalIdx := c.GetGlobalIndex(exportRef.LocalName); globalIdx >= 0 {
					exportIndices[exportName] = globalIdx
					// fmt.Printf("// [Compiler] GetExportGlobalIndices: Re-export '%s' maps to local global[%d]\n", exportName, globalIdx)
				}
			}
		} else {
			// Local export: The export name should correspond to the local name in globals
			if globalIdx := c.GetGlobalIndex(exportRef.LocalName); globalIdx >= 0 {
				exportIndices[exportName] = globalIdx
				// fmt.Printf("// [Compiler] GetExportGlobalIndices: Local export '%s' maps to global[%d]\n", exportName, globalIdx)
			}
		}
	}

	// Handle default export if it exists
	if c.moduleBindings.DefaultExport != nil {
		defaultLocalName := c.moduleBindings.DefaultExport.LocalName
		if globalIdx := c.GetGlobalIndex(defaultLocalName); globalIdx >= 0 {
			exportIndices["default"] = globalIdx
			// fmt.Printf("// [Compiler] GetExportGlobalIndices: Default export '%s' maps to global[%d]\n", defaultLocalName, globalIdx)
		}
	}

	return exportIndices
}

// hasMethodInType checks if an object type has a method with the given name
// This includes checking the object's own properties and any prototype chain
func (c *Compiler) hasMethodInType(objType *types.ObjectType, methodName string) bool {
	// Check own properties
	if _, exists := objType.Properties[methodName]; exists {
		return true
	}

	// Check in base types (prototypes)
	for _, baseType := range objType.BaseTypes {
		if baseObjType, ok := baseType.(*types.ObjectType); ok {
			if c.hasMethodInType(baseObjType, methodName) {
				return true
			}
		}
	}

	return false
}

// --- Module Compilation Methods ---

// compileImportDeclaration handles compilation of import statements
// Following the same pattern as type checker's checkImportDeclaration
func (c *Compiler) compileImportDeclaration(node *parser.ImportDeclaration, hint Register) (Register, errors.PaseratiError) {
	// Import declarations are only valid at module top-level
	// If processedModules is nil, we're in a nested function scope
	if c.processedModules == nil {
		return BadRegister, NewCompileError(node, "import declarations can only appear at the top level of a module")
	}

	// Type-only imports are handled during type checking only - no runtime code needed
	if node.IsTypeOnly {
		debugPrintf("// [Compiler] Skipping type-only import from: %s\n", node.Source.Value)
		return BadRegister, nil
	}

	if node.Source == nil {
		return BadRegister, NewCompileError(node, "import statement missing source module")
	}

	sourceModulePath := node.Source.Value
	debugPrintf("// [Compiler] Processing import from: %s\n", sourceModulePath)

	// Check if this is a JSON module import
	isJSONModule := node.Attributes != nil && node.Attributes["type"] == "json"
	if isJSONModule {
		debugPrintf("// [Compiler] Detected JSON module import: %s\n", sourceModulePath)
		return c.compileJSONImport(node, sourceModulePath)
	}

	// Generate OpEvalModule to ensure the module is loaded and executed
	// Only emit if this module hasn't been processed yet
	if !c.processedModules[sourceModulePath] {
		debugPrintf("// [Compiler] Generating OpEvalModule for import from: %s\n", sourceModulePath)
		c.emitEvalModule(sourceModulePath, node.Token.Line)
		c.processedModules[sourceModulePath] = true
	} else {
		debugPrintf("// [Compiler] Module %s already processed, skipping OpEvalModule\n", sourceModulePath)
	}

	// Handle bare imports (side-effect only)
	if len(node.Specifiers) == 0 {
		debugPrintf("// [Compiler] Bare import (side effects only): %s\n", sourceModulePath)
		// For bare imports, we just need to ensure the module was loaded
		// No bindings are created in the local environment
		return BadRegister, nil
	}

	// Process import specifiers and create bindings
	for _, spec := range node.Specifiers {
		switch importSpec := spec.(type) {
		case *parser.ImportDefaultSpecifier:
			// Default import: import defaultName from "module"
			if importSpec.Local == nil {
				return BadRegister, NewCompileError(node, "import default specifier missing local name")
			}

			localName := importSpec.Local.Value
			c.processImportBinding(localName, sourceModulePath, "default", ImportDefaultRef)

		case *parser.ImportNamedSpecifier:
			// Named import: import { name } or import { name as alias }
			if importSpec.Local == nil || importSpec.Imported == nil {
				return BadRegister, NewCompileError(node, "import named specifier missing names")
			}

			localName := importSpec.Local.Value
			importedName := importSpec.Imported.Value
			c.processImportBinding(localName, sourceModulePath, importedName, ImportNamedRef)

		case *parser.ImportNamespaceSpecifier:
			// Namespace import: import * as name from "module"
			if importSpec.Local == nil {
				return BadRegister, NewCompileError(node, "import namespace specifier missing local name")
			}

			localName := importSpec.Local.Value
			c.processImportBinding(localName, sourceModulePath, "*", ImportNamespaceRef)

		default:
			return BadRegister, NewCompileError(node, fmt.Sprintf("unknown import specifier type: %T", spec))
		}
	}

	return BadRegister, nil
}

// compileJSONImport handles compilation of JSON module imports
// JSON modules have a synthetic default export containing the parsed JSON data
func (c *Compiler) compileJSONImport(node *parser.ImportDeclaration, sourceModulePath string) (Register, errors.PaseratiError) {
	// JSON modules can only have default imports
	// Emit OpLoadJSONModule to load and parse the JSON file
	c.emitLoadJSONModule(sourceModulePath, node.Token.Line)

	// Mark as processed so we don't try to eval it as a regular module
	c.processedModules[sourceModulePath] = true

	// Check if we're in module mode
	if !c.IsModuleMode() || c.moduleBindings == nil {
		return BadRegister, NewCompileError(node, "JSON module imports require module mode")
	}

	// Process import specifiers - JSON modules only support default import
	for _, spec := range node.Specifiers {
		switch importSpec := spec.(type) {
		case *parser.ImportDefaultSpecifier:
			// Default import: import data from "./file.json" with { type: "json" }
			if importSpec.Local == nil {
				return BadRegister, NewCompileError(node, "import default specifier missing local name")
			}

			localName := importSpec.Local.Value
			// For JSON modules, use module exports (OpGetModuleExport) not globals
			// We manually define the import binding with GlobalIndex = -1
			c.moduleBindings.DefineImport(localName, sourceModulePath, "default", ImportDefaultRef, -1)

		case *parser.ImportNamedSpecifier:
			// Named import: import { default as data } from "./file.json" with { type: "json" }
			// JSON modules only have a "default" export, so only allow named import of "default"
			if importSpec.Imported == nil {
				return BadRegister, NewCompileError(node, "import named specifier missing imported name")
			}
			if importSpec.Local == nil {
				return BadRegister, NewCompileError(node, "import named specifier missing local name")
			}

			// Only allow importing "default"
			if importSpec.Imported.Value != "default" {
				return BadRegister, NewCompileError(node, fmt.Sprintf("JSON modules only have a 'default' export, cannot import '%s'", importSpec.Imported.Value))
			}

			localName := importSpec.Local.Value
			// Import the default export with the specified local name
			c.moduleBindings.DefineImport(localName, sourceModulePath, "default", ImportDefaultRef, -1)

		case *parser.ImportNamespaceSpecifier:
			// Namespace import: import * as ns from "./file.json" with { type: "json" }
			if importSpec.Local == nil {
				return BadRegister, NewCompileError(node, "import namespace specifier missing local name")
			}

			localName := importSpec.Local.Value
			// For JSON modules with namespace import, use module exports
			c.moduleBindings.DefineImport(localName, sourceModulePath, "*", ImportNamespaceRef, -1)

		default:
			return BadRegister, NewCompileError(node, fmt.Sprintf("unknown import specifier type: %T", spec))
		}
	}

	return BadRegister, nil
}

// processImportBinding handles the binding of an imported name
// Parallels type checker's processImportBinding
func (c *Compiler) processImportBinding(localName, sourceModule, sourceName string, importType ImportReferenceType) {
	// If we're in module mode, use the module bindings for proper tracking
	if c.IsModuleMode() {
		// Since we're using a unified heap, the import should resolve to the same global index
		// as the export. The source name (what we're importing) should have a global index.
		globalIdx := c.GetGlobalIndex(sourceName)
		if globalIdx == -1 {
			// If not found, assign a new index (this coordinates the global index across modules)
			globalIdx = int(c.GetOrAssignGlobalIndex(sourceName))
		}
		c.moduleBindings.DefineImport(localName, sourceModule, sourceName, importType, globalIdx)

		// Try to resolve the actual value from the source module
		resolvedValue := c.moduleBindings.ResolveImportedValue(localName)
		if resolvedValue != vm.Undefined {
			debugPrintf("// [Compiler] Imported %s: %s = %d (resolved)\n", localName, sourceName, int(resolvedValue.Type()))
		} else {
			debugPrintf("// [Compiler] Imported %s: %s = undefined (unresolved)\n", localName, sourceName)
		}

		// Don't define imported names in the symbol table
		// They will be resolved via emitImportResolve when referenced
		// This ensures OpGetModuleExport is used instead of OpGetGlobal
	} else {
		// Fallback: just define the name in the symbol table
		c.currentSymbolTable.Define(localName, nilRegister)
		debugPrintf("// [Compiler] Imported %s: %s = undefined (no module mode)\n", localName, sourceName)
	}
}

// compileExportNamedDeclaration handles compilation of named export statements
// Parallels type checker's checkExportNamedDeclaration
func (c *Compiler) compileExportNamedDeclaration(node *parser.ExportNamedDeclaration, hint Register) (Register, errors.PaseratiError) {
	// Type-only exports are handled during type checking only - no runtime code needed
	if node.IsTypeOnly {
		debugPrintf("// [Compiler] Skipping type-only export\n")
		return BadRegister, nil
	}

	if node.Declaration != nil {
		// Direct export: export const x = 1; export function foo() {}
		debugPrintf("// [Compiler] Processing direct export declaration\n")

		// Compile the declaration first
		_, err := c.compileNode(node.Declaration, hint)
		if err != nil {
			return BadRegister, err
		}

		// Extract and register exported names
		c.processExportDeclaration(node.Declaration)

	} else if len(node.Specifiers) > 0 {
		// Named exports: export { x, y } or export { x } from "module"
		debugPrintf("// [Compiler] Processing named export specifiers\n")

		if node.Source != nil {
			// Re-export: export { x } from "module"
			sourceModule := node.Source.Value
			debugPrintf("// [Compiler] Re-export from: %s\n", sourceModule)

			for _, spec := range node.Specifiers {
				if exportSpec, ok := spec.(*parser.ExportNamedSpecifier); ok {
					localName := getExportSpecName(exportSpec.Local)
					exportName := getExportSpecName(exportSpec.Exported)

					if c.IsModuleMode() {
						c.moduleBindings.DefineReExport(exportName, sourceModule, localName)
						debugPrintf("// [Compiler] Re-exported: %s as %s from %s\n", localName, exportName, sourceModule)
					}
				}
			}
		} else {
			// Local export: export { x, y }
			// Validate that the exported names exist and register them
			for _, spec := range node.Specifiers {
				if exportSpec, ok := spec.(*parser.ExportNamedSpecifier); ok {
					if exportSpec.Local == nil {
						return BadRegister, NewCompileError(node, "export specifier missing local name")
					}

					localName := getExportSpecName(exportSpec.Local)
					exportName := getExportSpecName(exportSpec.Exported)

					// Check if the local name exists in current symbol table
					if _, _, exists := c.currentSymbolTable.Resolve(localName); exists {
						// Register the export in module bindings
						if c.IsModuleMode() {
							// For now, we can't get the runtime value until execution
							// So we register it with undefined and resolve later
							// Get the global index for this export
							globalIdx := c.GetGlobalIndex(localName)
							if globalIdx == -1 {
								globalIdx = int(c.GetOrAssignGlobalIndex(localName))
							}
							c.moduleBindings.DefineExport(localName, exportName, vm.Undefined, nil, globalIdx)
						}
						debugPrintf("// [Compiler] Exported: %s as %s\n", localName, exportName)
					} else {
						return BadRegister, NewCompileError(node, fmt.Sprintf("exported name '%s' not found in current scope", localName))
					}
				}
			}
		}
	}

	return BadRegister, nil
}

// compileExportDefaultDeclaration handles compilation of default export statements
// Parallels type checker's checkExportDefaultDeclaration
func (c *Compiler) compileExportDefaultDeclaration(node *parser.ExportDefaultDeclaration, hint Register) (Register, errors.PaseratiError) {
	if node.Declaration == nil {
		return BadRegister, NewCompileError(node, "export default statement missing declaration")
	}

	debugPrintf("// [Compiler] Processing default export\n")

	// Compile the default export expression
	resultReg, err := c.compileNode(node.Declaration, hint)
	if err != nil {
		return BadRegister, err
	}

	// Register the default export
	if c.IsModuleMode() {
		// Get or assign a global index for the default export
		globalIdx := c.GetOrAssignGlobalIndex("default")
		c.moduleBindings.DefineExport("default", "default", vm.Undefined, nil, int(globalIdx))

		// IMPORTANT: Store the compiled value to the global slot
		// This ensures the default export value is accessible via the module namespace
		c.emitSetGlobal(globalIdx, resultReg, node.Token.Line)

		debugPrintf("// [Compiler] Default export registered at global[%d]\n", globalIdx)
	} else {
		debugPrintf("// [Compiler] Default export processed (no module mode)\n")
	}

	return resultReg, nil
}

// compileExportAllDeclaration handles compilation of export all statements
// Transforms "export * from './module'" into equivalent individual exports
func (c *Compiler) compileExportAllDeclaration(node *parser.ExportAllDeclaration, hint Register) (Register, errors.PaseratiError) {
	if node.Source == nil {
		return BadRegister, NewCompileError(node, "export * statement missing source module")
	}

	sourceModule := node.Source.Value
	debugPrintf("// [Compiler] Processing export * from: %s\n", sourceModule)

	if !c.IsModuleMode() {
		debugPrintf("// [Compiler] Not in module mode, skipping re-export\n")
		return BadRegister, nil
	}

	// Get the source module to find its exports
	if c.moduleLoader == nil {
		debugPrintf("// [Compiler] No module loader available for re-export\n")
		return BadRegister, nil
	}

	sourceModuleRecord, err := c.moduleLoader.LoadModule(sourceModule, ".")
	if err != nil {
		return BadRegister, NewCompileError(node, fmt.Sprintf("Failed to load source module '%s' for re-export: %v", sourceModule, err))
	}

	// Get export names from the source module
	// Try runtime values first, then fall back to export names
	sourceExports := sourceModuleRecord.GetExportValues()
	exportNames := sourceModuleRecord.GetExportNames()

	debugPrintf("// [Compiler] Source module '%s' has %d runtime exports and %d export names\n", sourceModule, len(sourceExports), len(exportNames))

	// If we have runtime exports, use those names (most reliable)
	if len(sourceExports) > 0 {
		exportNames = nil // Clear the names array
		for exportName := range sourceExports {
			exportNames = append(exportNames, exportName)
		}
		debugPrintf("// [Compiler] Using runtime export names: %v\n", exportNames)
	} else if len(exportNames) > 0 {
		debugPrintf("// [Compiler] Using export names from module record: %v\n", exportNames)
	} else {
		debugPrintf("// [Compiler] No exports found in source module\n")
	}

	debugPrintf("// [Compiler] Will re-export %d names from '%s'\n", len(exportNames), sourceModule)

	// For each export in the source module, create an equivalent re-export
	// This transforms "export * from './math'" into:
	// 1. Generate import resolution for each export
	// 2. Store each imported value as a global (like normal exports do)
	for _, exportName := range exportNames {
		// Skip default exports for "export *" (TypeScript behavior)
		if exportName == "default" {
			debugPrintf("// [Compiler] Skipping default export '%s' in re-export all\n", exportName)
			continue
		}

		debugPrintf("// [Compiler] Re-exporting '%s' from '%s'\n", exportName, sourceModule)

		// Get/assign global index for this re-export
		globalIdx := int(c.GetOrAssignGlobalIndex(exportName))

		// 1. Define the import binding (like processImportDeclaration does)
		// Use -1 for GlobalIndex to force module export lookup instead of direct global access
		c.moduleBindings.DefineImport(exportName, sourceModule, exportName, ImportNamedRef, -1)

		// 2. Define the export binding (like processExportDeclaration does)
		c.moduleBindings.DefineExport(exportName, exportName, vm.Undefined, nil, globalIdx)

		// 3. Generate bytecode to import and re-export the value
		// Allocate a temporary register for the imported value
		tempReg := c.regAlloc.Alloc()

		// First ensure the source module is loaded
		c.emitEvalModule(sourceModule, node.Token.Line)

		// Then get the specific export from the source module
		c.emitGetModuleExport(tempReg, sourceModule, exportName, node.Token.Line)

		// Store the imported value as a global (like normal exports do)
		globalIdxUint16 := c.GetOrAssignGlobalIndex(exportName)
		c.emitSetGlobal(globalIdxUint16, tempReg, node.Token.Line)

		debugPrintf("// [Compiler] Stored re-exported '%s' as global at index %d\n", exportName, globalIdxUint16)

		// Free the temporary register
		c.regAlloc.Free(tempReg)
	}

	debugPrintf("// [Compiler] Completed re-export transformation for %d exports\n", len(exportNames))
	return BadRegister, nil
}

// extractExportNamesFromAST extracts export names from a module's AST
// This is used when runtime export values are not yet available
func (c *Compiler) extractExportNamesFromAST(program *parser.Program) []string {
	var exportNames []string

	if program == nil || program.Statements == nil {
		return exportNames
	}

	for _, stmt := range program.Statements {
		switch node := stmt.(type) {
		case *parser.ExportNamedDeclaration:
			if node.Declaration != nil {
				// Direct export: export const x = 1; export function foo() {}
				switch decl := node.Declaration.(type) {
				case *parser.LetStatement:
					if decl.Name != nil {
						exportNames = append(exportNames, decl.Name.Value)
					}
				case *parser.VarStatement:
					if decl.Name != nil {
						exportNames = append(exportNames, decl.Name.Value)
					}
				case *parser.ConstStatement:
					if decl.Name != nil {
						exportNames = append(exportNames, decl.Name.Value)
					}
				case *parser.ExpressionStatement:
					// Handle function declarations: export function foo() {}
					if expr, ok := decl.Expression.(*parser.FunctionLiteral); ok && expr.Name != nil {
						exportNames = append(exportNames, expr.Name.Value)
					}
				}
			} else if len(node.Specifiers) > 0 {
				// Named exports: export { x, y }
				for _, spec := range node.Specifiers {
					if exportSpec, ok := spec.(*parser.ExportNamedSpecifier); ok {
						if exportSpec.Local != nil {
							// Use the export name (or local name if no alias)
							exportName := getExportSpecName(exportSpec.Local)
							if exportSpec.Exported != nil {
								exportName = getExportSpecName(exportSpec.Exported)
							}
							exportNames = append(exportNames, exportName)
						}
					}
				}
			}
		case *parser.ExportDefaultDeclaration:
			exportNames = append(exportNames, "default")
		case *parser.ExportAllDeclaration:
			// Note: export * from "module" doesn't add any names directly
			// The names come from the source module
		}
	}

	return exportNames
}

// processExportDeclaration processes a declaration that's being exported directly
// Parallels type checker's processExportDeclaration
func (c *Compiler) processExportDeclaration(decl parser.Statement) {
	switch d := decl.(type) {
	case *parser.LetStatement:
		if c.IsModuleMode() {
			if d.Name != nil {
				// Let/const declarations also get stored as globals at top-level
				globalIdx := c.GetGlobalIndex(d.Name.Value)
				if globalIdx == -1 {
					globalIdx = int(c.GetOrAssignGlobalIndex(d.Name.Value))
				}
				c.moduleBindings.DefineExport(d.Name.Value, d.Name.Value, vm.Undefined, d, globalIdx)
				debugPrintf("// [Compiler] Exported let: %s at global[%d]\n", d.Name.Value, globalIdx)
			}
		}

	case *parser.VarStatement:
		if c.IsModuleMode() {
			if d.Name != nil {
				// Var declarations also get stored as globals at top-level
				globalIdx := c.GetGlobalIndex(d.Name.Value)
				if globalIdx == -1 {
					globalIdx = int(c.GetOrAssignGlobalIndex(d.Name.Value))
				}
				c.moduleBindings.DefineExport(d.Name.Value, d.Name.Value, vm.Undefined, d, globalIdx)
				debugPrintf("// [Compiler] Exported var: %s at global[%d]\n", d.Name.Value, globalIdx)
			}
		}

	case *parser.ConstStatement:
		if c.IsModuleMode() {
			if d.Name != nil {
				// Let/const declarations also get stored as globals at top-level
				globalIdx := c.GetGlobalIndex(d.Name.Value)
				if globalIdx == -1 {
					globalIdx = int(c.GetOrAssignGlobalIndex(d.Name.Value))
				}
				c.moduleBindings.DefineExport(d.Name.Value, d.Name.Value, vm.Undefined, d, globalIdx)
				debugPrintf("// [Compiler] Exported const: %s at global[%d]\n", d.Name.Value, globalIdx)
			}
		}

	case *parser.ExpressionStatement:
		// Handle function declarations in expression statements
		if expr, ok := d.Expression.(*parser.FunctionLiteral); ok && expr.Name != nil {
			if c.IsModuleMode() {
				// Function declarations are automatically stored as globals at top-level
				// Get the global index that was already assigned during function compilation
				globalIdx := c.GetGlobalIndex(expr.Name.Value)
				if globalIdx == -1 {
					// If not found, assign a new one (though this shouldn't happen for functions)
					globalIdx = int(c.GetOrAssignGlobalIndex(expr.Name.Value))
				}
				c.moduleBindings.DefineExport(expr.Name.Value, expr.Name.Value, vm.Undefined, d, globalIdx)
				debugPrintf("// [Compiler] Exported function: %s at global[%d]\n", expr.Name.Value, globalIdx)
			}
		}

	default:
		debugPrintf("// [Compiler] Unhandled export declaration type: %T\n", decl)
	}
}

// emitImportResolve generates code to resolve an imported name at runtime
// This generates OpEvalModule to execute the source module and make imports available
func (c *Compiler) emitImportResolve(destReg Register, importName string, line int) {
	if !c.IsModuleMode() {
		debugPrintf("// [Compiler] emitImportResolve: Not in module mode, loading undefined for '%s'\n", importName)
		c.emitLoadUndefined(destReg, line)
		return
	}

	// Get the import reference for this name
	if importRef, exists := c.moduleBindings.ImportedNames[importName]; exists {
		// Handle namespace imports specially
		if importRef.ImportType == ImportNamespaceRef && importRef.SourceName == "*" {
			debugPrintf("// [Compiler] emitImportResolve: Handling namespace import for '%s'\n", importName)
			// For namespace imports, we need to create the namespace object from module exports
			// TODO: This might need special handling for namespace creation
			c.emitCreateNamespace(destReg, importRef.SourceModule, line)
			// Also store it in the global for future access
			if importRef.GlobalIndex != -1 {
				c.emitSetGlobal(uint16(importRef.GlobalIndex), destReg, line)
			}
		} else if importRef.GlobalIndex != -1 {
			// Direct global access - imported values should already be loaded
			debugPrintf("// [Compiler] emitImportResolve: Using direct global access for '%s' at index %d\n",
				importName, importRef.GlobalIndex)
			c.emitGetGlobal(destReg, uint16(importRef.GlobalIndex), line)
		} else {
			// Fallback to module export lookup (for backwards compatibility)
			debugPrintf("// [Compiler] emitImportResolve: Fallback to module export lookup for '%s'\n", importName)
			c.emitGetModuleExport(destReg, importRef.SourceModule, importRef.SourceName, line)
		}
	} else {
		debugPrintf("// [Compiler] emitImportResolve: No import binding found for '%s', loading undefined\n", importName)
		c.emitLoadUndefined(destReg, line)
	}
}

// emitEvalModule generates OpEvalModule bytecode to execute a module
func (c *Compiler) emitEvalModule(modulePath string, line int) {
	// Add module path as a string constant
	modulePathValue := vm.String(modulePath)
	constantIdx := c.chunk.AddConstant(modulePathValue)

	debugPrintf("// [Compiler] emitEvalModule: Emitting OpEvalModule for '%s' (constantIdx: %d)\n", modulePath, constantIdx)

	// Emit OpEvalModule with the constant index
	c.emitOpCode(vm.OpEvalModule, line)
	c.emitByte(byte(constantIdx >> 8))   // High byte
	c.emitByte(byte(constantIdx & 0xFF)) // Low byte
}

// emitLoadJSONModule emits bytecode to load a JSON module
// JSON modules use the same OpEvalModule but are handled specially in the VM
func (c *Compiler) emitLoadJSONModule(modulePath string, line int) {
	// JSON modules use the same loading mechanism as regular modules
	// The VM will detect that it's a JSON module and handle it appropriately
	c.emitEvalModule(modulePath, line)
}

// emitCreateNamespace generates bytecode to create a namespace object from module exports
func (c *Compiler) emitCreateNamespace(destReg Register, modulePath string, line int) {
	// Add module path as a string constant
	modulePathValue := vm.String(modulePath)
	modulePathIdx := c.chunk.AddConstant(modulePathValue)

	debugPrintf("// [Compiler] emitCreateNamespace: R%d = createNamespace('%s')\n", destReg, modulePath)

	// Emit OpCreateNamespace with the destination register and module path constant
	c.emitOpCode(vm.OpCreateNamespace, line)
	c.emitByte(byte(destReg))
	c.emitByte(byte(modulePathIdx >> 8))   // High byte
	c.emitByte(byte(modulePathIdx & 0xFF)) // Low byte
}

// emitGetModuleExport generates OpGetModuleExport bytecode to get an exported value
func (c *Compiler) emitGetModuleExport(destReg Register, modulePath string, exportName string, line int) {
	// Add module path as a string constant
	modulePathValue := vm.String(modulePath)
	modulePathIdx := c.chunk.AddConstant(modulePathValue)

	// Add export name as a string constant
	exportNameValue := vm.String(exportName)
	exportNameIdx := c.chunk.AddConstant(exportNameValue)

	debugPrintf("// [Compiler] emitGetModuleExport: R%d = module['%s']['%s']\n", destReg, modulePath, exportName)

	// Emit OpGetModuleExport with register and two constant indices
	c.emitOpCode(vm.OpGetModuleExport, line)
	c.emitByte(byte(destReg))
	c.emitByte(byte(modulePathIdx >> 8))   // Module path high byte
	c.emitByte(byte(modulePathIdx & 0xFF)) // Module path low byte
	c.emitByte(byte(exportNameIdx >> 8))   // Export name high byte
	c.emitByte(byte(exportNameIdx & 0xFF)) // Export name low byte
}

// emitIteratorCleanup emits bytecode to call iterator.return() if it exists
// This is used for proper cleanup when for...of loops exit early (break, return, throw)
func (c *Compiler) emitIteratorCleanup(iteratorReg Register, line int) {
	// We need to check if iterator.return exists and call it if present
	// This is done defensively - we don't want cleanup to throw errors

	// Get iterator.return method (may be undefined)
	returnMethodReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(returnMethodReg)

	returnConstIdx := c.chunk.AddConstant(vm.String("return"))
	c.emitGetProp(returnMethodReg, iteratorReg, returnConstIdx, line)

	// Check if return method exists (not undefined/null)
	// We'll use OpIsNullish to check if it's null or undefined
	isNullishReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(isNullishReg)
	c.emitOpCode(vm.OpIsNullish, line)
	c.emitByte(byte(isNullishReg))
	c.emitByte(byte(returnMethodReg))

	// Negate the nullish result so we can use JumpIfFalse to skip when nullish
	notNullishReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(notNullishReg)
	c.emitOpCode(vm.OpNot, line)
	c.emitByte(byte(notNullishReg))
	c.emitByte(byte(isNullishReg))

	// Skip calling return if it's nullish (jump if NOT not-nullish = jump if nullish)
	skipCallJump := c.emitPlaceholderJump(vm.OpJumpIfFalse, notNullishReg, line)

	// Call iterator.return()
	resultReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(resultReg)

	c.emitCallMethod(resultReg, returnMethodReg, iteratorReg, 0, line)

	// Per ECMAScript spec 7.4.6 IteratorClose step 9:
	// If Type(innerResult.[[value]]) is not Object, throw a TypeError
	c.emitTypeGuardIteratorReturn(resultReg, line)

	// Patch the skip jump
	c.patchJump(skipCallJump)
}

// emitIteratorCleanupWithDone emits bytecode to call iterator.return() if it exists AND iterator is not done
// Per ECMAScript spec, IteratorClose should only be called if iteratorRecord.[[done]] is false
func (c *Compiler) emitIteratorCleanupWithDone(iteratorReg Register, doneReg Register, line int) {
	// First check if iterator is done - if done is true, skip cleanup entirely
	// We need to negate done so we can use JumpIfFalse (jump if NOT done)
	notDoneReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(notDoneReg)
	c.emitOpCode(vm.OpNot, line)
	c.emitByte(byte(notDoneReg))
	c.emitByte(byte(doneReg))

	// Skip all cleanup if done is true (i.e., if NOT-done is false)
	skipAllJump := c.emitPlaceholderJump(vm.OpJumpIfFalse, notDoneReg, line)

	// Iterator is NOT done, proceed with normal cleanup
	// Get iterator.return method (may be undefined)
	returnMethodReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(returnMethodReg)

	returnConstIdx := c.chunk.AddConstant(vm.String("return"))
	c.emitGetProp(returnMethodReg, iteratorReg, returnConstIdx, line)

	// Check if return method exists (not undefined/null)
	isNullishReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(isNullishReg)
	c.emitOpCode(vm.OpIsNullish, line)
	c.emitByte(byte(isNullishReg))
	c.emitByte(byte(returnMethodReg))

	// Negate the nullish result so we can use JumpIfFalse to skip when nullish
	notNullishReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(notNullishReg)
	c.emitOpCode(vm.OpNot, line)
	c.emitByte(byte(notNullishReg))
	c.emitByte(byte(isNullishReg))

	// Skip calling return if it's nullish
	skipCallJump := c.emitPlaceholderJump(vm.OpJumpIfFalse, notNullishReg, line)

	// Call iterator.return()
	resultReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(resultReg)
	c.emitCallMethod(resultReg, returnMethodReg, iteratorReg, 0, line)

	// Per ECMAScript spec 7.4.6 IteratorClose step 9:
	// If Type(innerResult.[[value]]) is not Object, throw a TypeError
	c.emitTypeGuardIteratorReturn(resultReg, line)

	// Patch the skip call jump
	c.patchJump(skipCallJump)

	// Patch the skip all jump (when done is true)
	c.patchJump(skipAllJump)
}

// emitTypeGuardIterable emits bytecode to check if a value is iterable
// Throws TypeError at runtime if not iterable
func (c *Compiler) emitTypeGuardIterable(valueReg Register, line int) {
	c.emitOpCode(vm.OpTypeGuardIterable, line)
	c.emitByte(byte(valueReg))
}

// emitTypeGuardIteratorReturn emits bytecode to check if iterator.return() result is an object
// Per ECMAScript spec 7.4.6 IteratorClose step 9
func (c *Compiler) emitTypeGuardIteratorReturn(resultReg Register, line int) {
	c.emitOpCode(vm.OpTypeGuardIteratorReturn, line)
	c.emitByte(byte(resultReg))
}

// getNextAnonymousId generates a unique ID for anonymous classes
func (c *Compiler) getNextAnonymousId() int {
	c.anonymousClassCounter++
	return c.anonymousClassCounter
}

// compileClassExpression compiles a class expression and returns the constructor function
// For named classes, it also stores them in the environment like class declarations
func (c *Compiler) compileClassExpression(node *parser.ClassDeclaration, hint Register) (Register, errors.PaseratiError) {
	debugPrintf("// DEBUG compileClassExpression: Starting compilation for class '%s'\n", node.Name.Value)

	// Per ECMAScript spec (sec-runtime-semantics-classdefinitionevaluation):
	// Named class expressions have an immutable binding of the class name
	// visible inside the class body (constructor and methods) but NOT outside.
	// The TDZ case (class x extends x {}) is handled explicitly below.
	// NOTE: At global level, use global variables. Inside functions, use spill slots
	// to avoid register exhaustion when many class expressions exist.
	// IMPORTANT: If the class name was inferred from an assignment target (e.g., let C = class {}),
	// we should NOT create an inner binding - only explicitly named classes get inner bindings.
	var prevSymbolTable *SymbolTable
	var classNameSpillSlot uint16
	var useSpillSlot bool
	// Check if this is a named class expression that needs an inner binding
	// Skip anonymous classes and classes with inferred names (from assignment targets like "let C = class {}")
	isNamedClassExpr := node.Name.Value != "" &&
		!strings.HasPrefix(node.Name.Value, "__AnonymousClass_") &&
		!strings.HasPrefix(node.Name.Value, "__Inferred__")
	if isNamedClassExpr {
		prevSymbolTable = c.currentSymbolTable
		c.currentSymbolTable = NewEnclosedSymbolTable(c.currentSymbolTable)
		// Use spill slot to avoid register exhaustion (works at both global and function level)
		classNameSpillSlot = c.AllocSpillSlot()
		useSpillSlot = true
		c.currentSymbolTable.DefineSpilledStrictImmutable(node.Name.Value, classNameSpillSlot)
		debugPrintf("// DEBUG compileClassExpression: Created inner binding for class '%s' in spill slot %d\n", node.Name.Value, classNameSpillSlot)
	}

	// Resolve super class INSIDE the inner scope so closures in heritage clause
	// capture the inner class name binding (per spec step 6a)
	var superConstructorReg Register = BadRegister
	var needToFreeSuperReg bool
	var cachedProtoReg Register = BadRegister
	var needToFreeCachedProtoReg bool
	if node.SuperClass != nil {
		if _, isNull := node.SuperClass.(*parser.NullLiteral); !isNull {
			// Check if it's an Identifier or GenericTypeRef - we can resolve by name
			var superClassName string
			var isNamedRef bool
			if ident, ok := node.SuperClass.(*parser.Identifier); ok {
				superClassName = ident.Value
				isNamedRef = true
			} else if genericTypeRef, ok := node.SuperClass.(*parser.GenericTypeRef); ok {
				superClassName = genericTypeRef.Name.Value
				isNamedRef = true
			}

			if isNamedRef {
				// Check for TDZ self-reference: class x extends x {}
				// This must throw ReferenceError per ECMAScript spec
				if superClassName == node.Name.Value {
					// Emit code to throw ReferenceError at runtime
					c.emitTDZError(BadRegister, superClassName, node.Token.Line)
					// We still need a register for the code flow, but it will never be used
					superConstructorReg = c.regAlloc.Alloc()
					needToFreeSuperReg = true
				} else {
					// Look up the parent class constructor using symbol table
					symbol, defTable, exists := c.currentSymbolTable.Resolve(superClassName)
					if exists {
						if symbol.IsGlobal {
							superConstructorReg = c.regAlloc.Alloc()
							needToFreeSuperReg = true
							c.emitGetGlobal(superConstructorReg, symbol.GlobalIndex, node.Token.Line)
						} else if symbol.IsSpilled {
							superConstructorReg = c.regAlloc.Alloc()
							needToFreeSuperReg = true
							c.emitLoadSpill(superConstructorReg, symbol.SpillIndex, node.Token.Line)
						} else if !c.isInCurrentScopeChain(defTable) && c.enclosing != nil {
							// Symbol from enclosing function's scope - compile as expression
							// for proper upvalue access through the closure mechanism
							superConstructorReg = c.regAlloc.Alloc()
							needToFreeSuperReg = true
							_, err := c.compileNode(node.SuperClass, superConstructorReg)
							if err != nil {
								if prevSymbolTable != nil {
									c.currentSymbolTable = prevSymbolTable
								}
								c.regAlloc.Free(superConstructorReg)
								return BadRegister, err
							}
						} else {
							superConstructorReg = symbol.Register
							needToFreeSuperReg = false
						}
					} else {
						// Not in symbol table - might be a built-in class
						globalIdx := c.GetOrAssignGlobalIndex(superClassName)
						superConstructorReg = c.regAlloc.Alloc()
						needToFreeSuperReg = true
						c.emitGetGlobal(superConstructorReg, globalIdx, node.Token.Line)
					}
				}
			} else {
				// For arbitrary expressions, compile the expression to get the value at runtime
				superConstructorReg = c.regAlloc.Alloc()
				needToFreeSuperReg = true
				_, err := c.compileNode(node.SuperClass, superConstructorReg)
				if err != nil {
					if prevSymbolTable != nil {
						c.currentSymbolTable = prevSymbolTable
					}
					c.regAlloc.Free(superConstructorReg)
					return BadRegister, err
				}
			}
		}
	}

	// Emit runtime validation that the superclass is a valid constructor
	// Per ECMAScript: must be callable with [[Construct]], or null
	// The VM will throw TypeError if invalid (e.g., arrow functions)
	// OpValidateSuperclass also caches the prototype value to avoid duplicate access
	if superConstructorReg != BadRegister {
		cachedProtoReg = c.regAlloc.Alloc()
		needToFreeCachedProtoReg = true
		c.emitOpCode(vm.OpValidateSuperclass, node.Token.Line)
		c.emitByte(byte(superConstructorReg))
		c.emitByte(byte(cachedProtoReg))
	}

	// Enter class brand context for private field tracking
	// Each class gets a unique brand ID to distinguish its private fields from other classes
	c.enterClassBrand()
	defer c.exitClassBrand()

	// Declare all private fields this class has
	c.declareClassPrivateNames(node)

	// Pre-evaluate computed field keys at class definition time
	// Per ECMAScript, computed property keys ([expr]) must be evaluated when the class
	// is defined, not when instances are created
	if err := c.preEvaluateComputedFieldKeys(node); err != nil {
		if prevSymbolTable != nil {
			c.currentSymbolTable = prevSymbolTable
		}
		if needToFreeSuperReg {
			c.regAlloc.Free(superConstructorReg)
		}
		return BadRegister, err
	}

	// 1. Create constructor function
	constructorReg, err := c.compileConstructor(node, BadRegister)
	if err != nil {
		if prevSymbolTable != nil {
			c.currentSymbolTable = prevSymbolTable
		}
		return BadRegister, err
	}

	// Store the constructor value into the class name binding (spill slot)
	if useSpillSlot {
		c.emitStoreSpill(classNameSpillSlot, constructorReg, node.Token.Line)
	}

	// 1b. Set up [[Prototype]] chain for derived class expressions
	if superConstructorReg != BadRegister {
		c.emitOpCode(vm.OpSetClosureProto, node.Token.Line)
		c.emitByte(byte(constructorReg))
		c.emitByte(byte(superConstructorReg))
		debugPrintf("// DEBUG compileClassExpression: Set constructor's [[Prototype]] to parent class\n")
	}

	// 2. Set up prototype object with methods
	err = c.setupClassPrototype(node, constructorReg, superConstructorReg, cachedProtoReg)
	if err != nil {
		return BadRegister, err
	}

	if needToFreeSuperReg {
		c.regAlloc.Free(superConstructorReg)
	}
	if needToFreeCachedProtoReg {
		c.regAlloc.Free(cachedProtoReg)
	}

	// 3. Set up static members on the constructor
	err = c.setupStaticMembers(node, constructorReg)
	if err != nil {
		return BadRegister, err
	}

	// 4. Restore outer scope (inner class name scope is no longer needed)
	// This must happen before defining outer bindings (step 4b)
	if prevSymbolTable != nil {
		c.currentSymbolTable = prevSymbolTable
	}

	// NOTE: Using spill slots for class name bindings avoids register exhaustion.
	// Spill slots are captured correctly by closures via CaptureFromSpill.

	// 4b. For named classes used as standalone expressions (hint == BadRegister),
	// define them in the outer environment like class declarations.
	// IMPORTANT: If a hint register is provided, the class is being used as an expression value
	// (e.g., default parameter value), so don't define it in the symbol table - the variable
	// is already defined and we're just assigning the class value to it.
	// Also skip if the name was inferred from assignment target (isNamedClassExpr is false).
	if hint == BadRegister && isNamedClassExpr {
		if c.enclosing == nil {
			// Top-level class - define as global
			globalIdx := c.GetOrAssignGlobalIndex(node.Name.Value)
			c.currentSymbolTable.DefineGlobal(node.Name.Value, globalIdx)
			c.emitSetGlobal(globalIdx, constructorReg, node.Token.Line)
			debugPrintf("// DEBUG compileClassExpression: Defined global class '%s' at index %d\n", node.Name.Value, globalIdx)
		} else {
			// Local class - define in local scope
			c.currentSymbolTable.Define(node.Name.Value, constructorReg)
			debugPrintf("// DEBUG compileClassExpression: Defined local class '%s' in R%d\n", node.Name.Value, constructorReg)
		}
	}

	// 5. Always return the constructor register for expressions
	// If a hint register was provided and it's different from constructorReg, move the value
	if hint != BadRegister && hint != constructorReg {
		c.emitMove(hint, constructorReg, node.Token.Line)
		c.regAlloc.Free(constructorReg)
		debugPrintf("// DEBUG compileClassExpression: Moved constructor from R%d to hint R%d\n", constructorReg, hint)
		return hint, nil
	}

	debugPrintf("// DEBUG compileClassExpression: Returning constructor in R%d\n", constructorReg)
	return constructorReg, nil
}
