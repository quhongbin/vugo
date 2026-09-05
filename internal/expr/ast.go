// Package expr 实现类 Vue 表达式到 html/template 表达式的转译。
//
// 它是模板转译的一个"子语言"层：自带词法（scanner.go）、语法（parser.go）、
// 作用域（scope.go）与代码生成（render.go），不依赖模板 AST，
// 因此可以独立测试与复用。
package expr

// Expr 是表达式 AST 的公共接口。
type Expr interface {
	exprNode()
	pos() int
}

// Ident 是一个变量引用，如 user 或 user.name.first。
type Ident struct {
	Name string   // 根标识，如 "user"
	Path []string // 后续字段路径，如 ["name", "first"]
	at   int
}

func (*Ident) exprNode()  {}
func (e *Ident) pos() int { return e.at }

// Call 是一个函数调用，如 formatTime(time)。
type Call struct {
	Func string // 函数名，可含点号，如 "strings.ToUpper"
	Args []Expr
	at   int
}

func (*Call) exprNode()  {}
func (e *Call) pos() int { return e.at }

// Literal 是一个字面量，Value 为可直接输出的 Go 模板源码。
type Literal struct {
	Value string
	at    int
}

func (*Literal) exprNode()  {}
func (e *Literal) pos() int { return e.at }

// Paren 是一个括号表达式，用于保持原有运算优先级。
type Paren struct {
	X  Expr
	at int
}

func (*Paren) exprNode()  {}
func (e *Paren) pos() int { return e.at }

// Unary 是一元运算，如 !ok。
type Unary struct {
	Op string
	X  Expr
	at int
}

func (*Unary) exprNode()  {}
func (e *Unary) pos() int { return e.at }

// Binary 是二元运算，如 a && b。
type Binary struct {
	Op string
	L  Expr
	R  Expr
	at int
}

func (*Binary) exprNode()  {}
func (e *Binary) pos() int { return e.at }
