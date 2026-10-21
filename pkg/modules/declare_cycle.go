package modules

import (
	"fmt"
	"runtime/debug"

	paseratiErrors "github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/types"
)

// EarlyExporter is implemented by a TypeChecker that can answer for its
// module's exports while it is still checking it.
type EarlyExporter interface {
	EarlyExport(name string) (types.Type, bool)
	EarlyExportNames() []string
	// SetDeclaredHook makes Check call hook once the module's declarations
	// are in place, before function bodies and statements are checked.
	SetDeclaredHook(hook func())
}

// exposeEarlyExports lets importers see record's exports while checker
// checks it.
func exposeEarlyExports(record *ModuleRecord, checker TypeChecker) {
	if early, ok := checker.(EarlyExporter); ok {
		record.EarlyExport, record.EarlyExportNames = early.EarlyExport, early.EarlyExportNames
	}
}

// declaredCheck is a module's Check running ahead of its turn on its own
// goroutine, paused once its declarations are in place. Only one of it and
// the goroutine that started it runs at a time.
type declaredCheck struct {
	checker TypeChecker
	paused  chan struct{}
	resume  chan struct{}
	done    chan struct{}
	errs    []paseratiErrors.PaseratiError
	failure any
}

// DeclareModule starts checking record, a module that is loaded but not yet
// checked, ahead of its turn: a module it imports, directly or not, imports
// it back and is being checked first. It returns once record's declarations
// are in place, which importers then see through record.EarlyExport; the
// loader finishes the check when it reaches record.
func (ml *moduleLoader) DeclareModule(record *ModuleRecord) {
	if record == nil || record.declared != nil || record.EarlyExport != nil || record.Exports != nil ||
		record.AST == nil || record.isNative || record.IsJSON || (record.State != ModuleResolved && record.State != ModuleParsed) ||
		ml.checkerFactory == nil || ml.config.SkipTypeCheck {
		return
	}
	checker := ml.checkerFactory()
	early, ok := checker.(EarlyExporter)
	if !ok {
		return
	}
	checker.EnableModuleMode(record.ResolvedPath, ml)
	d := &declaredCheck{
		checker: checker,
		paused:  make(chan struct{}),
		resume:  make(chan struct{}),
		done:    make(chan struct{}),
	}
	early.SetDeclaredHook(func() {
		d.paused <- struct{}{}
		<-d.resume
	})
	record.declared = d
	exposeEarlyExports(record, checker)
	program := record.AST
	go func() {
		defer func() {
			if p := recover(); p != nil {
				d.failure = fmt.Sprintf("%v\n%s", p, debug.Stack())
			}
			close(d.done)
		}()
		d.errs = checker.Check(program)
	}()
	select {
	case <-d.paused:
	case <-d.done:
		d.rethrow()
	}
}

// finish resumes the paused check and returns its diagnostics.
func (d *declaredCheck) finish() []paseratiErrors.PaseratiError {
	close(d.resume)
	<-d.done
	d.rethrow()
	return d.errs
}

func (d *declaredCheck) rethrow() {
	if d.failure != nil {
		panic(d.failure)
	}
}
