package parser

import (
	"bytes"
	"fmt"
	"strings"
)

// JSEmitter is responsible for transforming AST nodes into JavaScript code
type JSEmitter struct {
	indentLevel int
	buffer      bytes.Buffer
}

// NewJSEmitter creates a new JavaScript emitter
func NewJSEmitter() *JSEmitter {
	return &JSEmitter{
		indentLevel: 0,
	}
}

// Emit converts a program AST to JavaScript code
func (e *JSEmitter) Emit(program *Program) string {
	e.buffer.Reset()
	e.indentLevel = 0

	for _, stmt := range program.Statements {
		e.emitStatement(stmt)
	}

	return e.buffer.String()
}

// Helper methods

func (e *JSEmitter) indent() {
	e.indentLevel++
}

func (e *JSEmitter) dedent() {
	if e.indentLevel > 0 {
		e.indentLevel--
	}
}

func (e *JSEmitter) writeIndent() {
	for i := 0; i < e.indentLevel; i++ {
		e.buffer.WriteString("  ")
	}
}

func (e *JSEmitter) writeLine(format string, args ...interface{}) {
	e.writeIndent()
	fmt.Fprintf(&e.buffer, format, args...)
	e.buffer.WriteString("\n")
}

func (e *JSEmitter) write(format string, args ...interface{}) {
	fmt.Fprintf(&e.buffer, format, args...)
}

// AST emitter methods

func (e *JSEmitter) emitStatement(stmt Statement) {
	switch s := stmt.(type) {
	case *LetStatement:
		e.emitLetStatement(s)
	case *VarStatement:
		e.emitVarStatement(s)
	case *ConstStatement:
		e.emitConstStatement(s)
	case *ReturnStatement:
		e.emitReturnStatement(s)
	case *ExpressionStatement:
		e.emitExpressionStatement(s)
	case *BlockStatement:
		e.emitBlockStatement(s)
	case *WhileStatement:
		e.emitWhileStatement(s)
	case *ForStatement:
		e.emitForStatement(s)
	case *DoWhileStatement:
		e.emitDoWhileStatement(s)
	case *BreakStatement:
		e.emitBreakStatement(s)
	case *ContinueStatement:
		e.emitContinueStatement(s)
	case *SwitchStatement:
		e.emitSwitchStatement(s)
	case *TypeAliasStatement:
		// Skip TypeAliasStatement as it's TS-specific
	default:
		// Handle unknown statement types
		e.writeLine("/* Unsupported statement type: %T */", s)
	}
}

func (e *JSEmitter) emitLetStatement(stmt *LetStatement) {
	e.writeIndent()
	e.write("let %s", stmt.Name.Value)

	if stmt.Value != nil {
		e.write(" = ")
		e.emitExpression(stmt.Value)
	}

	e.write(";\n")
}

func (e *JSEmitter) emitVarStatement(stmt *VarStatement) {
	e.writeIndent()
	e.write("var %s", stmt.Name.Value)

	if stmt.Value != nil {
		e.write(" = ")
		e.emitExpression(stmt.Value)
	}

	e.write(";\n")
}

func (e *JSEmitter) emitConstStatement(stmt *ConstStatement) {
	e.writeIndent()
	e.write("const %s", stmt.Name.Value)

	if stmt.Value != nil {
		e.write(" = ")
		e.emitExpression(stmt.Value)
	}

	e.write(";\n")
}

func (e *JSEmitter) emitReturnStatement(stmt *ReturnStatement) {
	e.writeIndent()
	e.write("return")

	if stmt.ReturnValue != nil {
		e.write(" ")
		e.emitExpression(stmt.ReturnValue)
	}

	e.write(";\n")
}

func (e *JSEmitter) emitExpressionStatement(stmt *ExpressionStatement) {
	e.writeIndent()

	// Special handling for expressions that can also work as statements
	switch expr := stmt.Expression.(type) {
	case *IfExpression:
		e.emitIfStatement(expr)
		return
	default:
		e.emitExpression(stmt.Expression)
		e.write(";\n")
	}
}

// emitIfStatement handles IfExpression when appearing as a statement
func (e *JSEmitter) emitIfStatement(expr *IfExpression) {
	e.write("if (")
	e.emitExpression(expr.Condition)
	e.write(") ")
	e.emitBlockStatement(expr.Consequence)

	if expr.Alternative != nil {
		e.write("else ")
		e.emitBlockStatement(expr.Alternative)
	}
}

func (e *JSEmitter) emitBlockStatement(stmt *BlockStatement) {
	e.writeLine("{")
	e.indent()

	for _, s := range stmt.Statements {
		e.emitStatement(s)
	}

	e.dedent()
	e.writeIndent()
	e.write("}\n")
}

func (e *JSEmitter) emitWhileStatement(stmt *WhileStatement) {
	e.writeIndent()
	e.write("while (")
	e.emitExpression(stmt.Condition)
	e.write(") ")
	e.emitBlockStatement(stmt.Body)
}

func (e *JSEmitter) emitForStatement(stmt *ForStatement) {
	e.writeIndent()
	e.write("for (")

	// Initializer
	if stmt.Initializer != nil {
		switch init := stmt.Initializer.(type) {
		case *LetStatement:
			e.write("let %s", init.Name.Value)
			if init.Value != nil {
				e.write(" = ")
				e.emitExpression(init.Value)
			}
		case *VarStatement:
			e.write("var %s", init.Name.Value)
			if init.Value != nil {
				e.write(" = ")
				e.emitExpression(init.Value)
			}
		case *ExpressionStatement:
			e.emitExpression(init.Expression)
		}
	}

	e.write("; ")

	// Condition
	if stmt.Condition != nil {
		e.emitExpression(stmt.Condition)
	}

	e.write("; ")

	// Update
	if stmt.Update != nil {
		e.emitExpression(stmt.Update)
	}

	e.write(") ")
	e.emitBlockStatement(stmt.Body)
}

func (e *JSEmitter) emitDoWhileStatement(stmt *DoWhileStatement) {
	e.writeIndent()
	e.write("do ")
	e.emitBlockStatement(stmt.Body)
	e.writeIndent()
	e.write("while (")
	e.emitExpression(stmt.Condition)
	e.write(");\n")
}

func (e *JSEmitter) emitBreakStatement(_ *BreakStatement) {
	e.writeLine("break;")
}

func (e *JSEmitter) emitContinueStatement(_ *ContinueStatement) {
	e.writeLine("continue;")
}

func (e *JSEmitter) emitSwitchStatement(stmt *SwitchStatement) {
	e.writeIndent()
	e.write("switch (")
	e.emitExpression(stmt.Expression)
	e.writeLine(") {")
	e.indent()

	for _, c := range stmt.Cases {
		if c.Condition != nil {
			e.writeIndent()
			e.write("case ")
			e.emitExpression(c.Condition)
			e.writeLine(":")
		} else {
			e.writeLine("default:")
		}

		e.indent()
		for _, s := range c.Body.Statements {
			e.emitStatement(s)
		}
		e.dedent()
	}

	e.dedent()
	e.writeLine("}")
}

func (e *JSEmitter) emitExpression(expr Expression) {
	switch exp := expr.(type) {
	case *Identifier:
		e.write(exp.Value)
	case *BooleanLiteral:
		e.write(fmt.Sprintf("%t", exp.Value))
	case *NumberLiteral:
		e.write(exp.TokenLiteral())
	case *StringLiteral:
		e.write(exp.TokenLiteral())
	case *NullLiteral:
		e.write("null")
	case *UndefinedLiteral:
		e.write("undefined")
	case *FunctionLiteral:
		e.emitFunctionLiteral(exp)
	case *ArrowFunctionLiteral:
		e.emitArrowFunctionLiteral(exp)
	case *CallExpression:
		e.emitCallExpression(exp)
	case *PrefixExpression:
		e.emitPrefixExpression(exp)
	case *InfixExpression:
		e.emitInfixExpression(exp)
	case *AssignmentExpression:
		e.emitAssignmentExpression(exp)
	case *UpdateExpression:
		e.emitUpdateExpression(exp)
	case *TernaryExpression:
		e.emitTernaryExpression(exp)
	case *ArrayLiteral:
		e.emitArrayLiteral(exp)
	case *IndexExpression:
		e.emitIndexExpression(exp)
	case *MemberExpression:
		e.emitMemberExpression(exp)
	case *ObjectLiteral:
		e.emitObjectLiteral(exp)
	case *IfExpression:
		e.emitIfExpression(exp)
	default:
		// Handle unsupported expression types
		e.write("/* Unsupported expression type: %T */", exp)
	}
}

func (e *JSEmitter) emitFunctionLiteral(fn *FunctionLiteral) {
	e.write("function")

	if fn.Name != nil {
		e.write(" %s", fn.Name.Value)
	}

	e.write("(")

	params := []string{}
	for _, p := range fn.Parameters {
		params = append(params, p.Name.Value)
	}
	e.write(strings.Join(params, ", "))

	e.write(") ")
	e.emitBlockStatement(fn.Body)
}

func (e *JSEmitter) emitArrowFunctionLiteral(fn *ArrowFunctionLiteral) {
	params := []string{}
	for _, p := range fn.Parameters {
		params = append(params, p.Name.Value)
	}

	if len(params) == 1 {
		e.write(params[0])
	} else {
		e.write("(%s)", strings.Join(params, ", "))
	}

	e.write(" => ")

	switch body := fn.Body.(type) {
	case *BlockStatement:
		e.emitBlockStatement(body)
	case Expression:
		e.emitExpression(body)
	}
}

func (e *JSEmitter) emitCallExpression(call *CallExpression) {
	e.emitExpression(call.Function)

	e.write("(")

	for i, arg := range call.Arguments {
		e.emitExpression(arg)

		if i < len(call.Arguments)-1 {
			e.write(", ")
		}
	}

	e.write(")")
}

func (e *JSEmitter) emitPrefixExpression(expr *PrefixExpression) {
	e.write("(%s", expr.Operator)
	e.emitExpression(expr.Right)
	e.write(")")
}

func (e *JSEmitter) emitInfixExpression(expr *InfixExpression) {
	e.write("(")
	e.emitExpression(expr.Left)
	e.write(" %s ", expr.Operator)
	e.emitExpression(expr.Right)
	e.write(")")
}

func (e *JSEmitter) emitAssignmentExpression(expr *AssignmentExpression) {
	e.emitExpression(expr.Left)
	e.write(" %s ", expr.Operator)
	e.emitExpression(expr.Value)
}

func (e *JSEmitter) emitUpdateExpression(expr *UpdateExpression) {
	if expr.Prefix {
		e.write("%s", expr.Operator)
		e.emitExpression(expr.Argument)
	} else {
		e.emitExpression(expr.Argument)
		e.write("%s", expr.Operator)
	}
}

func (e *JSEmitter) emitTernaryExpression(expr *TernaryExpression) {
	e.write("(")
	e.emitExpression(expr.Condition)
	e.write(" ? ")
	e.emitExpression(expr.Consequence)
	e.write(" : ")
	e.emitExpression(expr.Alternative)
	e.write(")")
}

func (e *JSEmitter) emitArrayLiteral(arr *ArrayLiteral) {
	e.write("[")

	for i, elem := range arr.Elements {
		e.emitExpression(elem)

		if i < len(arr.Elements)-1 {
			e.write(", ")
		}
	}

	e.write("]")
}

func (e *JSEmitter) emitIndexExpression(expr *IndexExpression) {
	e.emitExpression(expr.Left)
	e.write("[")
	e.emitExpression(expr.Index)
	e.write("]")
}

func (e *JSEmitter) emitMemberExpression(expr *MemberExpression) {
	e.emitExpression(expr.Object)
	e.write(".")
	e.write(expr.Property.Value)
}

func (e *JSEmitter) emitObjectLiteral(obj *ObjectLiteral) {
	e.write("{")

	if len(obj.Properties) > 0 {
		e.write(" ")

		for i, prop := range obj.Properties {
			keyExpr, isIdent := prop.Key.(*Identifier)

			if isIdent {
				e.write(keyExpr.Value)
			} else {
				e.emitExpression(prop.Key)
			}

			e.write(": ")
			e.emitExpression(prop.Value)

			if i < len(obj.Properties)-1 {
				e.write(", ")
			}
		}

		e.write(" ")
	}

	e.write("}")
}

func (e *JSEmitter) emitIfExpression(expr *IfExpression) {
	e.write("((() => {")
	e.write(" if (")
	e.emitExpression(expr.Condition)
	e.write(") ")
	e.write("{ return ")
	// First statement from consequence
	if len(expr.Consequence.Statements) > 0 {
		if exprStmt, ok := expr.Consequence.Statements[0].(*ExpressionStatement); ok {
			e.emitExpression(exprStmt.Expression)
		} else {
			e.write("undefined")
		}
	} else {
		e.write("undefined")
	}
	e.write("; }")

	if expr.Alternative != nil {
		e.write(" else { return ")
		// First statement from alternative
		if len(expr.Alternative.Statements) > 0 {
			if exprStmt, ok := expr.Alternative.Statements[0].(*ExpressionStatement); ok {
				e.emitExpression(exprStmt.Expression)
			} else {
				e.write("undefined")
			}
		} else {
			e.write("undefined")
		}
		e.write("; }")
	}

	e.write(" })())")
}
