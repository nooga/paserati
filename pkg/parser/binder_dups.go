package parser

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/lexer"
)

// Duplicate-declaration diagnostics, ported from TypeScript's binder
// (declareSymbol and its symbol flags / excludes table) and the checker's
// duplicate-member rules.
//
// The pass is purely syntactic: it builds one symbol table per scope, declares
// every name with its SymbolFlags, and reports a conflict when the new
// declaration's `excludes` intersects the flags the existing symbol already
// has - on every earlier declaration of that symbol and on the new one, exactly
// like tsc. Which merges are legal (interface+class, namespace+enum, ...) falls
// out of the excludes table rather than being special-cased.
//
// Results go to Program.BindErrors (the type checker reports them); the JS
// early errors the parser already raised for the same positions are re-labelled
// with the TypeScript code in place (see finishBinding).

type symFlags uint32

const (
	sfFunctionScopedVariable symFlags = 1 << iota
	sfBlockScopedVariable
	sfProperty
	sfEnumMember
	sfFunction
	sfClass
	sfInterface
	sfConstEnum
	sfRegularEnum
	sfValueModule
	sfNamespaceModule
	sfMethod
	sfConstructor
	sfGetAccessor
	sfSetAccessor
	sfTypeParameter
	sfTypeAlias
	sfAlias
)

const (
	sfEnum     = sfConstEnum | sfRegularEnum
	sfVariable = sfFunctionScopedVariable | sfBlockScopedVariable
	sfValue    = sfVariable | sfProperty | sfEnumMember | sfFunction | sfClass | sfEnum | sfValueModule | sfMethod | sfGetAccessor | sfSetAccessor
	sfType     = sfClass | sfInterface | sfEnum | sfEnumMember | sfTypeParameter | sfTypeAlias

	exFunctionScopedVariable = sfValue &^ sfFunctionScopedVariable
	exBlockScopedVariable    = sfValue
	exParameter              = sfValue
	exProperty               = symFlags(0)
	exEnumMember             = sfValue | sfType
	exFunction               = sfValue &^ (sfFunction | sfValueModule | sfClass)
	exClass                  = (sfValue | sfType) &^ (sfValueModule | sfInterface | sfFunction)
	exInterface              = sfType &^ (sfInterface | sfClass)
	exRegularEnum            = (sfValue | sfType) &^ (sfRegularEnum | sfValueModule)
	exConstEnum              = (sfValue | sfType) &^ sfConstEnum
	exValueModule            = sfValue &^ (sfFunction | sfClass | sfRegularEnum | sfValueModule)
	exNamespaceModule        = symFlags(0)
	exMethod                 = sfValue &^ sfMethod
	exGetAccessor            = sfValue &^ sfSetAccessor
	exSetAccessor            = sfValue &^ sfGetAccessor
	exTypeParameter          = sfType &^ sfTypeParameter
	exTypeAlias              = sfType
	exAlias                  = sfAlias
	exAll                    = ^symFlags(0)
)

// bindDecl is one declaration of a symbol.
type bindDecl struct {
	tok       *lexer.Token // the declaration's name token (error position)
	kind      declKind
	hasBody   bool
	ambient   bool
	isStatic  bool
	isConst   bool // const enum
	isDefault bool // `export default` declaration
	pos       int
}

type declKind int

const (
	dkOther declKind = iota
	dkFunction
	dkClass
	dkMethod
	dkConstructor
	dkEnum
	dkNamespace
)

type bindSymbol struct {
	name    string
	flags   symFlags
	decls   []*bindDecl
	exports bindTable // static members / namespace exports / enum members
	members bindTable // instance members

	enumFirstMissing *bool // a declaration already omitted its first enum initializer
}

type bindTable map[string]*bindSymbol

func (s *bindSymbol) exportsTable() bindTable {
	if s.exports == nil {
		s.exports = bindTable{}
	}
	return s.exports
}

func (s *bindSymbol) membersTable() bindTable {
	if s.members == nil {
		s.members = bindTable{}
	}
	return s.members
}

// bindScope is the set of tables names declare into.
type bindScope struct {
	locals   bindTable // block-scoped container: let/const/class/function/enum/interface/type
	varTable bindTable // function/file/namespace body: var, namespace
	exports  bindTable // exported declarations (module files and namespace bodies); nil otherwise
}

type bindDiag struct {
	tok  *lexer.Token
	code string
	msg  string
}

type binder struct {
	// Names introduced by any type-level declaration, in any scope.
	typeNames map[string]bool
	diags     []bindDiag
	seen  map[string]bool
	syms  []*bindSymbol
	src   interface{}
}

func (b *binder) report(tok *lexer.Token, code, msg string) {
	if tok == nil {
		return
	}
	key := fmt.Sprintf("%d:%s", tok.StartPos, code)
	if b.seen[key] {
		return
	}
	b.seen[key] = true
	b.diags = append(b.diags, bindDiag{tok: tok, code: code, msg: msg})
}

// declare is TypeScript's declareSymbol for the non-export-merging cases.
func (b *binder) declare(table bindTable, name string, tok *lexer.Token, flags, excludes symFlags, d *bindDecl) *bindSymbol {
	if table == nil || name == "" || tok == nil {
		return &bindSymbol{name: name}
	}
	d.tok = tok
	d.pos = tok.StartPos
	if flags&(sfInterface|sfTypeAlias|sfClass|sfEnum|sfValueModule|sfNamespaceModule) != 0 {
		b.typeNames[name] = true
	}
	sym := table[name]
	if sym == nil {
		sym = &bindSymbol{name: name}
		table[name] = sym
		b.syms = append(b.syms, sym)
	} else if sym.flags&excludes != 0 {
		code, msg := tsCodeTwoThreeHundred, fmt.Sprintf("Duplicate identifier '%s'.", name)
		if sym.flags&sfBlockScopedVariable != 0 {
			code, msg = tsCodeCannotRedeclare, fmt.Sprintf("Cannot redeclare block-scoped variable '%s'.", name)
		}
		if sym.flags&sfEnum != 0 || flags&sfEnum != 0 {
			code, msg = tsCodeEnumMerge, "Enum declarations can only merge with namespace or other enum declarations."
		}
		if d.isDefault && len(sym.decls) > 0 {
			code, msg = "TS2528", "A module cannot have multiple default exports."
		}
		for _, prev := range sym.decls {
			b.report(prev.tok, code, msg)
		}
		b.report(tok, code, msg)
		// Like tsc, the conflicting declaration joins a fresh orphan symbol
		// and does not change the table.
		orphan := &bindSymbol{name: name, flags: flags, decls: []*bindDecl{d}}
		return orphan
	}
	sym.flags |= flags
	sym.decls = append(sym.decls, d)
	return sym
}

const (
	tsCodeTwoThreeHundred = "TS2300"
	tsCodeCannotRedeclare = "TS2451"
	tsCodeEnumMerge       = "TS2567"
)

// --- Program entry ---------------------------------------------------------

// bindDeclarations runs the duplicate-declaration analysis over a parsed
// program and returns its diagnostics.
func bindDeclarations(program *Program) []bindDiag {
	b := &binder{seen: map[string]bool{}, typeNames: map[string]bool{}}
	isModule := false
	for _, s := range program.Statements {
		switch s.(type) {
		case *ImportDeclaration, *ExportNamedDeclaration, *ExportDefaultDeclaration, *ExportAllDeclaration:
			isModule = true
		}
	}
	locals := bindTable{}
	sc := &bindScope{locals: locals, varTable: locals}
	if isModule {
		sc.exports = bindTable{}
	}
	b.bindList(program.Statements, sc)
	b.finish()
	program.DeclaredTypeNames = b.typeNames
	return b.diags
}

// finish runs the symbol-level checks that need every declaration of a symbol:
// duplicate implementations, class/function merges, namespace placement.
func (b *binder) finish() {
	for _, sym := range b.syms {
		b.checkFunctionLike(sym)
		b.checkNamespacePlacement(sym)
		b.checkEnumDeclarations(sym)
	}
}

func (b *binder) checkFunctionLike(sym *bindSymbol) {
	var fnLike []*bindDecl
	bodies := 0
	ctorBodies := 0
	hasNonAmbientClass := false
	for _, d := range sym.decls {
		switch d.kind {
		case dkFunction, dkMethod:
			fnLike = append(fnLike, d)
			if d.hasBody {
				bodies++
			}
		case dkConstructor:
			fnLike = append(fnLike, d)
			if d.hasBody {
				ctorBodies++
			}
		case dkClass:
			if !d.ambient {
				hasNonAmbientClass = true
			}
		}
	}
	if ctorBodies > 1 {
		for _, d := range fnLike {
			if d.kind == dkConstructor {
				b.report(d.tok, "TS2392", "Multiple constructor implementations are not allowed.")
			}
		}
	}
	if bodies > 1 {
		for _, d := range fnLike {
			if d.kind != dkConstructor {
				b.report(d.tok, "TS2393", "Duplicate function implementation.")
			}
		}
	}
	if hasNonAmbientClass && sym.flags&sfFunction != 0 {
		for _, d := range sym.decls {
			switch d.kind {
			case dkClass:
				b.report(d.tok, "TS2813", fmt.Sprintf("Class declaration cannot implement overload list for '%s'.", sym.name))
			case dkFunction:
				b.report(d.tok, "TS2814", "Function with bodies can only merge with classes that are ambient.")
			}
		}
	}
}

// checkNamespacePlacement reports TS2434: an instantiated namespace declared
// before the class or function it merges with.
func (b *binder) checkNamespacePlacement(sym *bindSymbol) {
	if sym.flags&sfValueModule == 0 || len(sym.decls) < 2 {
		return
	}
	first := -1
	for _, d := range sym.decls {
		if (d.kind == dkClass || d.kind == dkFunction) && !d.ambient {
			if first < 0 || d.pos < first {
				first = d.pos
			}
		}
	}
	if first < 0 {
		return
	}
	for _, d := range sym.decls {
		if d.kind == dkNamespace && !d.ambient && d.pos < first {
			b.report(d.tok, "TS2434", "A namespace declaration cannot be located prior to a class or function with which it is merged.")
		}
	}
}

// checkEnumDeclarations reports TS2473: all declarations of an enum must be
// const or all non-const.
func (b *binder) checkEnumDeclarations(sym *bindSymbol) {
	var enums []*bindDecl
	for _, d := range sym.decls {
		if d.kind == dkEnum {
			enums = append(enums, d)
		}
	}
	if len(enums) < 2 {
		return
	}
	sort.SliceStable(enums, func(i, j int) bool { return enums[i].pos < enums[j].pos })
	for _, d := range enums {
		if d.isConst != enums[0].isConst {
			b.report(d.tok, "TS2473", "Enum declarations must all be const or non-const.")
		}
	}
}

// --- Statement lists --------------------------------------------------------

func (b *binder) bindList(stmts []Statement, sc *bindScope) {
	// tsc binds function declarations first (bindEachFunctionsFirst), which is
	// observable: the flags of the first symbol decide the error message.
	for _, s := range stmts {
		if isFunctionDeclStmt(s) {
			b.bindStmt(s, sc)
		}
	}
	for _, s := range stmts {
		if !isFunctionDeclStmt(s) {
			b.bindStmt(s, sc)
		}
	}
}

func isFunctionDeclStmt(s Statement) bool {
	if e, ok := s.(*ExportNamedDeclaration); ok && e != nil {
		s = e.Declaration
	}
	if d, ok := s.(*ExportDefaultDeclaration); ok && d != nil {
		switch f := d.Declaration.(type) {
		case *FunctionLiteral:
			return f != nil && d.IsDeclaration
		case *FunctionSignature:
			return f != nil
		}
		return false
	}
	_, _, ok := functionDeclInfo(s)
	return ok
}

// functionDeclInfo recognises function declarations and overload signatures.
func functionDeclInfo(s Statement) (fl *FunctionLiteral, sig *FunctionSignature, ok bool) {
	switch n := s.(type) {
	case *ExpressionStatement:
		if n == nil {
			return nil, nil, false
		}
		switch e := n.Expression.(type) {
		case *FunctionLiteral:
			if e != nil && e.Name != nil && !e.Parenthesized {
				return e, nil, true
			}
		case *FunctionSignature:
			if e != nil && e.Name != nil {
				return nil, e, true
			}
		}
	case *FunctionSignature:
		if n != nil && n.Name != nil {
			return nil, n, true
		}
	}
	return nil, nil, false
}

func (b *binder) bindStmt(s Statement, sc *bindScope) {
	if s == nil {
		return
	}
	if e, ok := s.(*ExportNamedDeclaration); ok {
		if e != nil && e.Declaration != nil {
			b.bindDeclaration(e.Declaration, sc, true)
		}
		return
	}
	b.bindDeclaration(s, sc, false)
}

func (b *binder) tableFor(sc *bindScope, exported bool, own bindTable) bindTable {
	if exported && sc.exports != nil {
		return sc.exports
	}
	return own
}

func (b *binder) bindDeclaration(s Statement, sc *bindScope, exported bool) {
	if fl, sig, ok := functionDeclInfo(s); ok {
		b.bindFunctionDecl(fl, sig, sc, exported)
		return
	}
	switch n := s.(type) {
	case *ExportDefaultDeclaration:
		b.bindExportDefault(n, sc)
	case *LetStatement, *ConstStatement:
		names, _ := declaredNameIdents(n)
		for _, id := range names {
			b.declare(b.tableFor(sc, exported, sc.locals), id.Value, id.Token, sfBlockScopedVariable, exBlockScopedVariable, &bindDecl{})
		}
	case *VarStatement, *ArrayDestructuringDeclaration, *ObjectDestructuringDeclaration, *DeclarationGroup:
		names, isVar := declaredNameIdents(n)
		for _, id := range names {
			if isVar {
				b.declare(b.tableFor(sc, exported, sc.varTable), id.Value, id.Token, sfFunctionScopedVariable, exFunctionScopedVariable, &bindDecl{})
			} else {
				b.declare(b.tableFor(sc, exported, sc.locals), id.Value, id.Token, sfBlockScopedVariable, exBlockScopedVariable, &bindDecl{})
			}
		}
	case *ClassDeclaration:
		if n != nil {
			b.bindClassDecl(n, sc, exported)
		}
	case *InterfaceDeclaration:
		if n != nil && n.Name != nil {
			sym := b.declare(b.tableFor(sc, exported, sc.locals), n.Name.Value, n.Name.Token, sfInterface, exInterface, &bindDecl{})
			b.bindTypeParams(n.TypeParameters)
			b.bindInterfaceMembers(n, sym)
		}
	case *TypeAliasStatement:
		if n != nil && n.Name != nil {
			b.declare(b.tableFor(sc, exported, sc.locals), n.Name.Value, n.Name.Token, sfTypeAlias, exTypeAlias, &bindDecl{})
			b.bindTypeParams(n.TypeParameters)
		}
	case *NamespaceDeclaration:
		if n != nil {
			b.bindNamespace(n, sc, exported)
		}
	case *ExpressionStatement:
		if n == nil {
			return
		}
		if en, ok := n.Expression.(*EnumDeclaration); ok && en != nil {
			b.bindEnum(en, sc, exported)
		}
	case *BlockStatement:
		if n != nil {
			b.bindList(n.Statements, &bindScope{locals: bindTable{}, varTable: sc.varTable})
		}
	case *IfStatement:
		if n != nil {
			b.bindBlock(n.Consequence, sc)
			b.bindBlock(n.Alternative, sc)
		}
	case *WhileStatement:
		if n != nil {
			b.bindBlock(n.Body, sc)
		}
	case *DoWhileStatement:
		if n != nil {
			b.bindBlock(n.Body, sc)
		}
	case *ForStatement:
		if n != nil {
			inner := &bindScope{locals: bindTable{}, varTable: sc.varTable}
			b.bindLoopHead(n.Initializer, inner)
			b.bindBlock(n.Body, inner)
		}
	case *ForInStatement:
		if n != nil {
			inner := &bindScope{locals: bindTable{}, varTable: sc.varTable}
			b.bindLoopHead(n.Variable, inner)
			b.bindBlock(n.Body, inner)
		}
	case *ForOfStatement:
		if n != nil {
			inner := &bindScope{locals: bindTable{}, varTable: sc.varTable}
			b.bindLoopHead(n.Variable, inner)
			b.bindBlock(n.Body, inner)
		}
	case *LabeledStatement:
		if n != nil {
			b.bindDeclaration(n.Statement, sc, false)
		}
	case *WithStatement:
		if n != nil {
			b.bindDeclaration(n.Body, sc, false)
		}
	case *TryStatement:
		if n != nil {
			b.bindBlock(n.Body, sc)
			if n.CatchClause != nil {
				b.bindBlock(n.CatchClause.Body, sc)
			}
			b.bindBlock(n.FinallyBlock, sc)
		}
	case *SwitchStatement:
		if n != nil {
			// The whole CaseBlock is one block scope.
			inner := &bindScope{locals: bindTable{}, varTable: sc.varTable}
			for _, c := range n.Cases {
				if c != nil && c.Body != nil {
					b.bindList(c.Body.Statements, inner)
				}
			}
		}
	}
}

func (b *binder) bindBlock(blk *BlockStatement, sc *bindScope) {
	if blk == nil {
		return
	}
	b.bindList(blk.Statements, &bindScope{locals: bindTable{}, varTable: sc.varTable})
}

// bindLoopHead declares the bindings a for/for-in/for-of head introduces.
func (b *binder) bindLoopHead(head Statement, sc *bindScope) {
	switch head.(type) {
	case *LetStatement, *ConstStatement, *VarStatement, *ArrayDestructuringDeclaration, *ObjectDestructuringDeclaration, *DeclarationGroup:
		b.bindDeclaration(head, sc, false)
	}
}

// declaredNameIdents lists the identifiers a variable declaration statement
// binds and whether it is a `var`.
func declaredNameIdents(s Statement) (names []*Identifier, isVar bool) {
	add := func(id *Identifier) { names = append(names, id) }
	isVar = false
	switch n := s.(type) {
	case *LetStatement:
		for _, d := range n.Declarations {
			if d != nil && d.Name != nil {
				add(d.Name)
			}
		}
	case *ConstStatement:
		for _, d := range n.Declarations {
			if d != nil && d.Name != nil {
				add(d.Name)
			}
		}
	case *VarStatement:
		isVar = true
		for _, d := range n.Declarations {
			if d != nil && d.Name != nil {
				add(d.Name)
			}
		}
	case *ArrayDestructuringDeclaration:
		isVar = n.Token != nil && n.Token.Type == lexer.VAR
		forEachArrayPatternIdentifier(n.Elements, add)
	case *ObjectDestructuringDeclaration:
		isVar = n.Token != nil && n.Token.Type == lexer.VAR
		forEachObjectPatternIdentifier(n.Properties, n.RestProperty, add)
	case *DeclarationGroup:
		for _, inner := range n.Declarations {
			sub, v := declaredNameIdents(inner)
			names = append(names, sub...)
			isVar = isVar || v
		}
	}
	return names, isVar
}

// --- Functions ----------------------------------------------------------------

func (b *binder) bindFunctionDecl(fl *FunctionLiteral, sig *FunctionSignature, sc *bindScope, exported bool) {
	var name *Identifier
	hasBody := false
	ambient := false
	if fl != nil {
		name = fl.Name
		hasBody = fl.Body != nil
	} else if sig != nil {
		name = sig.Name
		ambient = sig.Declare
	}
	if name == nil {
		return
	}
	d := &bindDecl{kind: dkFunction, hasBody: hasBody, ambient: ambient}
	b.declare(b.tableFor(sc, exported, sc.locals), name.Value, name.Token, sfFunction, exFunction, d)
	if fl != nil {
		b.bindFunctionBody(fl.TypeParameters, fl.Parameters, fl.RestParameter, fl.Body, nil)
	} else if sig != nil {
		b.bindTypeParams(sig.TypeParameters)
		b.bindParamsOnly(sig.Parameters, sig.RestParameter)
	}
}

// bindFunctionBody binds a function's parameters and body in a fresh function
// scope. extra lets callers (constructors) hook in before the body is bound.
func (b *binder) bindFunctionBody(tps []*TypeParameter, params []*Parameter, rest *RestParameter, body *BlockStatement, extra func(*bindScope)) {
	b.bindTypeParams(tps)
	locals := bindTable{}
	sc := &bindScope{locals: locals, varTable: locals}
	b.declareParams(locals, params, rest)
	if extra != nil {
		extra(sc)
	}
	if body != nil {
		b.bindList(body.Statements, sc)
	}
}

func (b *binder) bindParamsOnly(params []*Parameter, rest *RestParameter) {
	b.declareParams(bindTable{}, params, rest)
}

func (b *binder) declareParams(table bindTable, params []*Parameter, rest *RestParameter) {
	add := func(id *Identifier) {
		if id != nil {
			b.declare(table, id.Value, id.Token, sfFunctionScopedVariable, exParameter, &bindDecl{})
		}
	}
	for _, p := range params {
		if p == nil || p.IsThis {
			continue
		}
		if p.Pattern != nil {
			forEachBindingIdentifier(p.Pattern, add)
		} else {
			add(p.Name)
		}
	}
	if rest != nil {
		if rest.Pattern != nil {
			forEachBindingIdentifier(rest.Pattern, add)
		} else {
			add(rest.Name)
		}
	}
}

// bindTypeParams reports a repeated name in one type parameter list (tsc's
// checkTypeParameterList: TS2300 on the repeat).
func (b *binder) bindTypeParams(tps []*TypeParameter) {
	if len(tps) < 2 {
		return
	}
	seen := map[string]bool{}
	for _, tp := range tps {
		if tp == nil || tp.Name == nil {
			continue
		}
		if seen[tp.Name.Value] {
			b.report(tp.Name.Token, "TS2300", fmt.Sprintf("Duplicate identifier '%s'.", tp.Name.Value))
		}
		seen[tp.Name.Value] = true
	}
}

// --- Classes ------------------------------------------------------------------

type classMember struct {
	pos      int
	tok      *lexer.Token
	name     string
	isStatic bool
	flags    symFlags
	excludes symFlags
	decl     *bindDecl
	meaning  int // checker addName meaning
	private  bool
}

const (
	dmGetAccessor = 1 << iota
	dmSetAccessor
	dmMethod
	dmPrivateStatic
	dmGetOrSet = dmGetAccessor | dmSetAccessor
)

func (b *binder) bindClassDecl(cd *ClassDeclaration, sc *bindScope, exported bool) {
	var sym *bindSymbol
	if cd.Name != nil {
		d := &bindDecl{kind: dkClass, ambient: cd.Declare}
		sym = b.declare(b.tableFor(sc, exported, sc.locals), cd.Name.Value, cd.Name.Token, sfClass, exClass, d)
	} else {
		sym = &bindSymbol{}
	}
	b.bindTypeParams(cd.TypeParameters)
	b.bindClassBody(cd.Body, sym, cd.Declare)
}

func propertyKeyName(key Expression) (name string, tok *lexer.Token, private bool) {
	switch k := key.(type) {
	case *Identifier:
		return k.Value, k.Token, false
	case *StringLiteral:
		if looksNumeric(k.Value) {
			return "", nil, false // "1" and 1.0 name the same property in tsc; we skip numeric names
		}
		return k.Value, k.Token, false
	case *NumberLiteral:
		return "", nil, false
	case *PrivateIdentifier:
		return k.Value, k.Token, true
	case *ComputedPropertyName:
		switch e := k.Expr.(type) {
		case *StringLiteral:
			return e.Value, e.Token, false
		case *NumberLiteral:
			return fmt.Sprintf("%v", e.Value), e.Token, false
		}
	}
	return "", nil, false
}

func looksNumeric(s string) bool {
	if s == "" {
		return false
	}
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

func (b *binder) bindClassBody(body *ClassBody, sym *bindSymbol, ambient bool) {
	if body == nil {
		return
	}
	var items []*classMember
	var ctors []*bindDecl
	instance := sym.membersTable()
	static := sym.exportsTable()

	for _, m := range body.Methods {
		if m == nil {
			continue
		}
		if m.Kind == "constructor" {
			tok := m.Token
			if id, ok := m.Key.(*Identifier); ok && id != nil {
				tok = id.Token
			}
			if tok == nil {
				continue
			}
			d := &bindDecl{kind: dkConstructor, hasBody: m.Value != nil && m.Value.Body != nil, ambient: ambient, tok: tok, pos: tok.StartPos}
			ctors = append(ctors, d)
			items = append(items, &classMember{pos: tok.StartPos, tok: tok, name: "", decl: d})
			continue
		}
		name, tok, private := propertyKeyName(m.Key)
		if tok == nil {
			continue
		}
		cm := &classMember{pos: tok.StartPos, tok: tok, name: name, isStatic: m.IsStatic, private: private}
		switch m.Kind {
		case "getter":
			cm.flags, cm.excludes, cm.meaning = sfGetAccessor, exGetAccessor, dmGetAccessor
		case "setter":
			cm.flags, cm.excludes, cm.meaning = sfSetAccessor, exSetAccessor, dmSetAccessor
		default:
			cm.flags, cm.excludes, cm.meaning = sfMethod, exMethod, dmMethod
			cm.decl = &bindDecl{kind: dkMethod, hasBody: m.Value != nil && m.Value.Body != nil, ambient: ambient, isStatic: m.IsStatic}
		}
		if cm.decl == nil {
			cm.decl = &bindDecl{ambient: ambient, isStatic: m.IsStatic}
		}
		items = append(items, cm)
	}
	for _, ms := range body.MethodSigs {
		if ms == nil {
			continue
		}
		name, tok, private := propertyKeyName(ms.Key)
		if tok == nil {
			continue
		}
		cm := &classMember{pos: tok.StartPos, tok: tok, name: name, isStatic: ms.IsStatic, private: private}
		switch ms.Kind {
		case "getter":
			cm.flags, cm.excludes, cm.meaning = sfGetAccessor, exGetAccessor, dmGetAccessor
			cm.decl = &bindDecl{ambient: ambient, isStatic: ms.IsStatic}
		case "setter":
			cm.flags, cm.excludes, cm.meaning = sfSetAccessor, exSetAccessor, dmSetAccessor
			cm.decl = &bindDecl{ambient: ambient, isStatic: ms.IsStatic}
		default:
			cm.flags, cm.excludes, cm.meaning = sfMethod, exMethod, dmMethod
			cm.decl = &bindDecl{kind: dkMethod, hasBody: false, ambient: ambient, isStatic: ms.IsStatic}
		}
		items = append(items, cm)
	}
	for _, cs := range body.ConstructorSigs {
		if cs == nil || cs.Token == nil {
			continue
		}
		d := &bindDecl{kind: dkConstructor, ambient: ambient, tok: cs.Token, pos: cs.Token.StartPos}
		ctors = append(ctors, d)
		items = append(items, &classMember{pos: cs.Token.StartPos, tok: cs.Token, decl: d})
	}
	for _, p := range body.Properties {
		if p == nil {
			continue
		}
		name, tok, private := propertyKeyName(p.Key)
		if tok == nil {
			continue
		}
		items = append(items, &classMember{pos: tok.StartPos, tok: tok, name: name, isStatic: p.IsStatic, private: private,
			flags: sfProperty, excludes: exProperty, meaning: dmGetOrSet, decl: &bindDecl{ambient: ambient, isStatic: p.IsStatic}})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].pos < items[j].pos })

	// Constructor parameter properties are instance properties declared at the
	// constructor's position.
	for _, m := range body.Methods {
		if m == nil || m.Kind != "constructor" || m.Value == nil {
			continue
		}
		for _, prm := range m.Value.Parameters {
			if prm != nil && prm.Name != nil && prm.Pattern == nil && (prm.IsPublic || prm.IsPrivate || prm.IsProtected || prm.IsReadonly) {
				items = append(items, &classMember{pos: prm.Name.Token.StartPos, tok: prm.Name.Token, name: prm.Name.Value,
					flags: sfProperty, excludes: exProperty, meaning: dmGetOrSet, decl: &bindDecl{ambient: ambient}})
			}
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].pos < items[j].pos })

	// Binder: declare into the member tables.
	for _, it := range items {
		if it.name == "" || it.private {
			continue
		}
		table := instance
		if it.isStatic {
			table = static
		}
		b.declare(table, it.name, it.tok, it.flags, it.excludes, it.decl)
	}

	// Checker: checkClassForDuplicateDeclarations.
	instanceNames := map[string]int{}
	staticNames := map[string]int{}
	privateNames := map[string]int{}
	for _, it := range items {
		if it.name == "" {
			continue
		}
		names := instanceNames
		meaning := it.meaning
		if it.private {
			names = privateNames
			if it.isStatic {
				meaning |= dmPrivateStatic
			}
		} else if it.isStatic {
			names = staticNames
		}
		prev, ok := names[it.name]
		if !ok {
			names[it.name] = meaning
			continue
		}
		if prev&dmPrivateStatic != meaning&dmPrivateStatic {
			b.report(it.tok, "TS18038", fmt.Sprintf("Duplicate identifier '%s'. Static and instance elements cannot share the same private name.", it.name))
			continue
		}
		prevIsMethod, isMethod := prev&dmMethod != 0, meaning&dmMethod != 0
		if prevIsMethod || isMethod {
			if prevIsMethod != isMethod {
				b.report(it.tok, "TS2300", fmt.Sprintf("Duplicate identifier '%s'.", it.name))
			}
		} else if prev&meaning&^dmPrivateStatic != 0 {
			b.report(it.tok, "TS2300", fmt.Sprintf("Duplicate identifier '%s'.", it.name))
		} else {
			names[it.name] = prev | meaning
		}
	}

	// Private names: a duplicate private name is a TS2300 on both (the binder
	// declares them in the same table as other members under a mangled name).
	privTable := bindTable{}
	for _, it := range items {
		if it.private && it.name != "" {
			b.declare(privTable, it.name, it.tok, it.flags, it.excludes, it.decl)
		}
	}

	// Bind method bodies and static blocks.
	for _, m := range body.Methods {
		if m == nil || m.Value == nil {
			continue
		}
		b.bindFunctionBody(m.Value.TypeParameters, m.Value.Parameters, m.Value.RestParameter, m.Value.Body, nil)
	}
	for _, blk := range body.StaticInitializers {
		if blk != nil {
			locals := bindTable{}
			b.bindList(blk.Statements, &bindScope{locals: locals, varTable: locals})
		}
	}
	// Constructor declarations of one class share a symbol for the
	// multiple-implementations check.
	if len(ctors) > 0 {
		ctorSym := &bindSymbol{name: "constructor", flags: sfConstructor}
		ctorSym.decls = ctors
		b.syms = append(b.syms, ctorSym)
	}
}

// --- Interfaces ---------------------------------------------------------------

func (b *binder) bindInterfaceMembers(n *InterfaceDeclaration, sym *bindSymbol) {
	members := sym.membersTable()
	seen := map[string]*lexer.Token{}
	for _, p := range n.Properties {
		if p == nil || p.IsIndexSignature || p.IsConstructorSignature {
			continue
		}
		var name string
		var tok *lexer.Token
		if p.Name != nil {
			name, tok = p.Name.Value, p.Name.Token
		} else if p.ComputedName != nil {
			name, tok, _ = propertyKeyName(&ComputedPropertyName{Expr: p.ComputedName})
		}
		// `readonly [key: string]: T` index signatures reach us as a property
		// named "readonly"; they are not property declarations.
		if name == "" || tok == nil || name == "readonly" {
			continue
		}
		if p.IsMethod {
			b.declare(members, name, tok, sfMethod, exMethod, &bindDecl{})
			continue
		}
		b.declare(members, name, tok, sfProperty, exProperty, &bindDecl{})
		// checkObjectTypeForDuplicateDeclarations: a repeated property
		// signature within one declaration is reported on both.
		if first, dup := seen[name]; dup {
			msg := fmt.Sprintf("Duplicate identifier '%s'.", name)
			b.report(first, "TS2300", msg)
			b.report(tok, "TS2300", msg)
		} else {
			seen[name] = tok
		}
	}
}

// --- Enums --------------------------------------------------------------------

func (b *binder) bindEnum(en *EnumDeclaration, sc *bindScope, exported bool) {
	if en.Name == nil {
		return
	}
	flags, excl := sfRegularEnum, exRegularEnum
	if en.IsConst {
		flags, excl = sfConstEnum, exConstEnum
	}
	d := &bindDecl{kind: dkEnum, isConst: en.IsConst}
	sym := b.declare(b.tableFor(sc, exported, sc.locals), en.Name.Value, en.Name.Token, flags, excl, d)
	members := sym.exportsTable()
	for _, m := range en.Members {
		if m == nil || m.Name == nil {
			continue
		}
		b.declare(members, m.Name.Value, m.Name.Token, sfEnumMember, exEnumMember, &bindDecl{})
	}
	// TS2432: only one declaration of a merged enum may omit the initializer
	// of its first member.
	if len(en.Members) > 0 && en.Members[0] != nil && en.Members[0].Name != nil {
		first := en.Members[0]
		if sym.enumFirstMissing == nil {
			sym.enumFirstMissing = new(bool)
		}
		if first.Value == nil {
			if *sym.enumFirstMissing {
				b.report(first.Name.Token, "TS2432", "In an enum with multiple declarations, only one declaration can omit an initializer for its first enum element.")
			} else {
				*sym.enumFirstMissing = true
			}
		}
	}
}

// --- Namespaces ---------------------------------------------------------------

// moduleInstantiated mirrors getModuleInstanceState != NonInstantiated: a
// namespace holding only interfaces, type aliases and non-instantiated nested
// namespaces has no runtime value.
// IsInstantiatedNamespace reports whether a namespace body declares anything with
// a runtime value.
func IsInstantiatedNamespace(body *BlockStatement) bool { return moduleInstantiated(body) }

func moduleInstantiated(body *BlockStatement) bool {
	if body == nil {
		return false
	}
	for _, s := range body.Statements {
		if e, ok := s.(*ExportNamedDeclaration); ok && e != nil {
			if e.Declaration == nil {
				continue
			}
			s = e.Declaration
		}
		switch n := s.(type) {
		case nil:
		case *InterfaceDeclaration, *TypeAliasStatement, *ImportDeclaration:
		case *NamespaceDeclaration:
			if n != nil && moduleInstantiated(n.Body) {
				return true
			}
		default:
			return true
		}
	}
	return false
}

func (b *binder) bindNamespace(n *NamespaceDeclaration, sc *bindScope, exported bool) {
	if n.Name == nil {
		return
	}
	if n.AmbientModule {
		// declare module "x" / declare global: its own world.
		locals := bindTable{}
		if n.Body != nil {
			b.bindList(n.Body.Statements, &bindScope{locals: locals, varTable: locals, exports: bindTable{}})
		}
		return
	}
	flags, excl := sfNamespaceModule, exNamespaceModule
	instantiated := moduleInstantiated(n.Body)
	if instantiated {
		flags, excl = sfValueModule, exValueModule
	}
	d := &bindDecl{kind: dkNamespace, ambient: n.Declare}
	sym := b.declare(b.tableFor(sc, exported || n.IsExported, sc.varTable), n.Name.Value, n.Name.Token, flags, excl, d)
	if n.Body == nil {
		return
	}
	locals := bindTable{}
	b.bindList(n.Body.Statements, &bindScope{locals: locals, varTable: locals, exports: sym.exportsTable()})
}

// --- export default -------------------------------------------------------------

// bindExportDefault declares `export default ...` as the "default" export. A
// default function or class merges like any declaration of that kind; anything
// else is an alias, which conflicts with every other default export.
func (b *binder) bindExportDefault(n *ExportDefaultDeclaration, sc *bindScope) {
	if n == nil || sc.exports == nil {
		return
	}
	tok := n.Token
	// `export default <expression>` is an ExportAssignment, which excludes every
	// other kind of declaration of the name "default".
	flags, excl := sfAlias, exAll
	d := &bindDecl{isDefault: true}
	switch e := n.Declaration.(type) {
	case *FunctionLiteral:
		if n.IsDeclaration && e != nil {
			flags, excl = sfFunction, exFunction
			d.kind, d.hasBody = dkFunction, e.Body != nil
			if e.Name != nil {
				tok = e.Name.Token
			}
		}
	case *FunctionSignature:
		if e != nil {
			flags, excl = sfFunction, exFunction
			d.kind = dkFunction
			if e.Name != nil {
				tok = e.Name.Token
			}
		}
	case *ClassExpression:
		if n.IsDeclaration && e != nil {
			flags, excl = sfClass, exClass
			d.kind = dkClass
			if e.Name != nil {
				tok = e.Name.Token
			}
		}
	case *Identifier:
		if e != nil && e.Token != nil {
			tok = e.Token
		}
	}
	b.declare(sc.exports, "default", tok, flags, excl, d)
}

// --- Parser integration ----------------------------------------------------------

// finishBinding runs the declaration analysis on a finished program. Parser
// early errors for the same positions (JS redeclaration SyntaxErrors) take on
// the TypeScript code and wording; every other diagnostic goes to
// program.BindErrors for the type checker to report.
func (p *Parser) finishBinding(program *Program) {
	diags := bindDeclarations(program)
	if len(diags) == 0 {
		return
	}
	// With real syntax errors the recovered tree can invent declarations (a
	// stray `, foo(x) {}` becoming a second declarator), so only the early-error
	// re-labelling below is trusted, not new diagnostics.
	syntaxErrors := len(p.errors) - len(p.redeclarationErrors)
	covered := map[int]bool{}
	for _, e := range p.redeclarationErrors {
		for _, d := range diags {
			if d.tok != nil && d.tok.StartPos == e.Position.StartPos && (d.code == "TS2300" || d.code == "TS2451") {
				e.ErrorCode = d.code
				e.Msg = d.msg
				covered[d.tok.StartPos] = true
			}
		}
	}
	for _, d := range diags {
		if syntaxErrors > 0 {
			break
		}
		if covered[d.tok.StartPos] && (d.code == "TS2300" || d.code == "TS2451") {
			continue
		}
		program.BindErrors = append(program.BindErrors, &errors.TypeError{
			Position: errors.Position{
				Line:     d.tok.Line,
				Column:   d.tok.Column,
				StartPos: d.tok.StartPos,
				EndPos:   d.tok.EndPos,
				Source:   p.source,
			},
			Msg:       d.msg,
			ErrorCode: d.code,
		})
	}
}
