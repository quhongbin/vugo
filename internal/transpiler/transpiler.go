// Package transpiler 是转译管道的第三层：把 AST 生成为 html/template 源码。
//
// 本层是"类 Vue 语法 → Go 语法"的唯一改写点：
//   - 插值与指令表达式交给 expr 包按作用域改写
//   - v-for 指令展开为 {{ range }}...{{ end }}（见 for.go）
//   - 无法映射的指令/绑定直接报错并给出替代方案（见 directive.go）
package transpiler

import (
	"fmt"
	"strings"

	"github.com/quhongbin/vugo/internal/ast"
	"github.com/quhongbin/vugo/internal/expr"
)

// Transpiler 遍历 AST 并输出 html/template 源码，同时维护变量作用域栈。
type Transpiler struct {
	out   strings.Builder
	scope *expr.Scope
}

// New 创建一个转译器。
func New() *Transpiler {
	return &Transpiler{scope: expr.NewScope()}
}

// Transpile 把模板 AST 转译为 html/template 源码。
func (t *Transpiler) Transpile(doc *ast.Document) (string, error) {
	t.out.Reset()
	for _, n := range doc.Nodes {
		if err := t.writeNode(n); err != nil {
			return "", err
		}
	}
	return t.out.String(), nil
}

// writeNode 输出单个 AST 节点。
func (t *Transpiler) writeNode(n ast.Node) error {
	switch v := n.(type) {
	case *ast.Text:
		t.out.WriteString(v.Value)
		return nil

	case *ast.GoAction:
		// 原生 html/template action 原样透传，保证已有模板生态不受破坏
		t.writeAction(v.Code)
		return nil

	case *ast.Interpolation:
		code, err := expr.Render(v.Expr, t.scope)
		if err != nil {
			return fmt.Errorf("vugo: 第 %d 行: %w", v.Line, err)
		}
		t.writeAction(code)
		return nil

	case *ast.Element:
		return t.writeElement(v)

	default:
		return fmt.Errorf("vugo: 无法处理的 AST 节点 %T", n)
	}
}

// writeElement 输出一个元素，必要时用 {{ range }}...{{ end }} 包裹。
func (t *Transpiler) writeElement(el *ast.Element) error {
	loop, err := resolveElement(el)
	if err != nil {
		return err
	}

	if loop != nil {
		// 可迭代对象在循环外的作用域中求值
		iterable, err := expr.Render(loop.Iterable, t.scope)
		if err != nil {
			return fmt.Errorf("vugo: 第 %d 行: v-for 表达式错误: %w", el.Line, err)
		}
		t.writeAction(loop.Header(iterable))
		t.scope.Push(loop.Frame)
	}

	t.out.WriteString("<" + el.Name)
	for _, a := range el.Attrs {
		t.out.WriteString(" " + a.Name)
		if a.Value != "" {
			t.out.WriteString(`="` + a.Value + `"`)
		}
	}

	switch {
	case el.SelfClosing:
		t.out.WriteString(" />")
	case el.Void():
		t.out.WriteString(">")
	default:
		t.out.WriteString(">")
		for _, child := range el.Children {
			if err := t.writeNode(child); err != nil {
				return err
			}
		}
		t.out.WriteString("</" + el.Name + ">")
	}

	if loop != nil {
		t.scope.Pop()
		t.writeAction("end")
	}
	return nil
}

// writeAction 输出一个 html/template action。
func (t *Transpiler) writeAction(code string) {
	t.out.WriteString("{{ " + code + " }}")
}
