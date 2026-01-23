package parser

// ASTArena provides arena-style allocation for AST nodes.
// Nodes are allocated from pre-grown slices, reducing GC pressure.
// Call Reset() between parses to reuse the arena's backing memory.
type ASTArena struct {
	identifiers       []Identifier
	numberLiterals    []NumberLiteral
	stringLiterals    []StringLiteral
	booleanLiterals   []BooleanLiteral
	blockStatements   []BlockStatement
	ifStatements      []IfStatement
	infixExpressions  []InfixExpression
	prefixExpressions []PrefixExpression
	callExpressions   []CallExpression
	memberExpressions []MemberExpression
	objectProperties  []ObjectProperty
	objectLiterals    []ObjectLiteral
	arrayLiterals     []ArrayLiteral
	returnStatements  []ReturnStatement
	letStatements     []LetStatement
	constStatements   []ConstStatement
	varStatements     []VarStatement
	functionLiterals  []FunctionLiteral
	arrowFunctions    []ArrowFunctionLiteral
	assignmentExprs   []AssignmentExpression
	ternaryExprs      []TernaryExpression
}

// NewASTArena creates a new arena with pre-allocated capacity.
func NewASTArena() *ASTArena {
	return &ASTArena{
		// Pre-allocate based on typical usage patterns
		identifiers:       make([]Identifier, 0, 256),
		numberLiterals:    make([]NumberLiteral, 0, 64),
		stringLiterals:    make([]StringLiteral, 0, 64),
		booleanLiterals:   make([]BooleanLiteral, 0, 32),
		blockStatements:   make([]BlockStatement, 0, 128),
		ifStatements:      make([]IfStatement, 0, 64),
		infixExpressions:  make([]InfixExpression, 0, 128),
		prefixExpressions: make([]PrefixExpression, 0, 32),
		callExpressions:   make([]CallExpression, 0, 128),
		memberExpressions: make([]MemberExpression, 0, 128),
		objectProperties:  make([]ObjectProperty, 0, 128),
		objectLiterals:    make([]ObjectLiteral, 0, 64),
		arrayLiterals:     make([]ArrayLiteral, 0, 64),
		returnStatements:  make([]ReturnStatement, 0, 64),
		letStatements:     make([]LetStatement, 0, 64),
		constStatements:   make([]ConstStatement, 0, 64),
		varStatements:     make([]VarStatement, 0, 32),
		functionLiterals:  make([]FunctionLiteral, 0, 64),
		arrowFunctions:    make([]ArrowFunctionLiteral, 0, 64),
		assignmentExprs:   make([]AssignmentExpression, 0, 64),
		ternaryExprs:      make([]TernaryExpression, 0, 32),
	}
}

// Reset clears the arena for reuse, keeping backing memory allocated.
func (a *ASTArena) Reset() {
	a.identifiers = a.identifiers[:0]
	a.numberLiterals = a.numberLiterals[:0]
	a.stringLiterals = a.stringLiterals[:0]
	a.booleanLiterals = a.booleanLiterals[:0]
	a.blockStatements = a.blockStatements[:0]
	a.ifStatements = a.ifStatements[:0]
	a.infixExpressions = a.infixExpressions[:0]
	a.prefixExpressions = a.prefixExpressions[:0]
	a.callExpressions = a.callExpressions[:0]
	a.memberExpressions = a.memberExpressions[:0]
	a.objectProperties = a.objectProperties[:0]
	a.objectLiterals = a.objectLiterals[:0]
	a.arrayLiterals = a.arrayLiterals[:0]
	a.returnStatements = a.returnStatements[:0]
	a.letStatements = a.letStatements[:0]
	a.constStatements = a.constStatements[:0]
	a.varStatements = a.varStatements[:0]
	a.functionLiterals = a.functionLiterals[:0]
	a.arrowFunctions = a.arrowFunctions[:0]
	a.assignmentExprs = a.assignmentExprs[:0]
	a.ternaryExprs = a.ternaryExprs[:0]
}

// Allocation methods - each returns a pointer to a zeroed node in the arena

func (a *ASTArena) NewIdentifier() *Identifier {
	a.identifiers = append(a.identifiers, Identifier{})
	return &a.identifiers[len(a.identifiers)-1]
}

func (a *ASTArena) NewNumberLiteral() *NumberLiteral {
	a.numberLiterals = append(a.numberLiterals, NumberLiteral{})
	return &a.numberLiterals[len(a.numberLiterals)-1]
}

func (a *ASTArena) NewStringLiteral() *StringLiteral {
	a.stringLiterals = append(a.stringLiterals, StringLiteral{})
	return &a.stringLiterals[len(a.stringLiterals)-1]
}

func (a *ASTArena) NewBooleanLiteral() *BooleanLiteral {
	a.booleanLiterals = append(a.booleanLiterals, BooleanLiteral{})
	return &a.booleanLiterals[len(a.booleanLiterals)-1]
}

func (a *ASTArena) NewBlockStatement() *BlockStatement {
	a.blockStatements = append(a.blockStatements, BlockStatement{})
	return &a.blockStatements[len(a.blockStatements)-1]
}

func (a *ASTArena) NewIfStatement() *IfStatement {
	a.ifStatements = append(a.ifStatements, IfStatement{})
	return &a.ifStatements[len(a.ifStatements)-1]
}

func (a *ASTArena) NewInfixExpression() *InfixExpression {
	a.infixExpressions = append(a.infixExpressions, InfixExpression{})
	return &a.infixExpressions[len(a.infixExpressions)-1]
}

func (a *ASTArena) NewPrefixExpression() *PrefixExpression {
	a.prefixExpressions = append(a.prefixExpressions, PrefixExpression{})
	return &a.prefixExpressions[len(a.prefixExpressions)-1]
}

func (a *ASTArena) NewCallExpression() *CallExpression {
	a.callExpressions = append(a.callExpressions, CallExpression{})
	return &a.callExpressions[len(a.callExpressions)-1]
}

func (a *ASTArena) NewMemberExpression() *MemberExpression {
	a.memberExpressions = append(a.memberExpressions, MemberExpression{})
	return &a.memberExpressions[len(a.memberExpressions)-1]
}

func (a *ASTArena) NewObjectProperty() *ObjectProperty {
	a.objectProperties = append(a.objectProperties, ObjectProperty{})
	return &a.objectProperties[len(a.objectProperties)-1]
}

func (a *ASTArena) NewObjectLiteral() *ObjectLiteral {
	a.objectLiterals = append(a.objectLiterals, ObjectLiteral{})
	return &a.objectLiterals[len(a.objectLiterals)-1]
}

func (a *ASTArena) NewArrayLiteral() *ArrayLiteral {
	a.arrayLiterals = append(a.arrayLiterals, ArrayLiteral{})
	return &a.arrayLiterals[len(a.arrayLiterals)-1]
}

func (a *ASTArena) NewReturnStatement() *ReturnStatement {
	a.returnStatements = append(a.returnStatements, ReturnStatement{})
	return &a.returnStatements[len(a.returnStatements)-1]
}

func (a *ASTArena) NewLetStatement() *LetStatement {
	a.letStatements = append(a.letStatements, LetStatement{})
	return &a.letStatements[len(a.letStatements)-1]
}

func (a *ASTArena) NewConstStatement() *ConstStatement {
	a.constStatements = append(a.constStatements, ConstStatement{})
	return &a.constStatements[len(a.constStatements)-1]
}

func (a *ASTArena) NewVarStatement() *VarStatement {
	a.varStatements = append(a.varStatements, VarStatement{})
	return &a.varStatements[len(a.varStatements)-1]
}

func (a *ASTArena) NewFunctionLiteral() *FunctionLiteral {
	a.functionLiterals = append(a.functionLiterals, FunctionLiteral{})
	return &a.functionLiterals[len(a.functionLiterals)-1]
}

func (a *ASTArena) NewArrowFunctionLiteral() *ArrowFunctionLiteral {
	a.arrowFunctions = append(a.arrowFunctions, ArrowFunctionLiteral{})
	return &a.arrowFunctions[len(a.arrowFunctions)-1]
}

func (a *ASTArena) NewAssignmentExpression() *AssignmentExpression {
	a.assignmentExprs = append(a.assignmentExprs, AssignmentExpression{})
	return &a.assignmentExprs[len(a.assignmentExprs)-1]
}

func (a *ASTArena) NewTernaryExpression() *TernaryExpression {
	a.ternaryExprs = append(a.ternaryExprs, TernaryExpression{})
	return &a.ternaryExprs[len(a.ternaryExprs)-1]
}
