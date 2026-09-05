package expr

import (
	"fmt"
	"strings"
)

// keywords 是类 Vue 表达式中的字面量关键字到 Go 模板字面量的映射。
var keywords = map[string]string{
	"true":  "true",
	"false": "false",
	"null":  "nil",
	"nil":   "nil",
}

// binaryFuncs 把类 Vue 的中缀比较/逻辑运算符映射为 html/template 的内置函数。
// html/template 不支持 `==`、`&&` 这类中缀写法，只接受 eq、and 等函数形式。
var binaryFuncs = map[string]string{
	"==": "eq", "!=": "ne",
	">": "gt", ">=": "ge", "<": "lt", "<=": "le",
	"&&": "and", "||": "or",
}

// Render 把一个类 Vue 表达式转译为 html/template 表达式。
//
// scope 为当前所在的作用域（v-for 循环变量绑定），传 nil 表示根作用域。
func Render(src string, scope *Scope) (string, error) {
	e, err := Parse(src)
	if err != nil {
		return "", err
	}
	if scope == nil {
		scope = NewScope()
	}
	return render(e, scope)
}

// render 生成单个表达式节点的模板源码。
func render(e Expr, s *Scope) (string, error) {
	switch v := e.(type) {
	case *Literal:
		return v.Value, nil

	case *Ident:
		return renderIdent(v, s), nil

	case *Paren:
		inner, err := render(v.X, s)
		if err != nil {
			return "", err
		}
		return "(" + inner + ")", nil

	case *Call:
		args := make([]string, 0, len(v.Args))
		for _, arg := range v.Args {
			r, err := render(arg, s)
			if err != nil {
				return "", err
			}
			args = append(args, r)
		}
		if len(args) == 0 {
			return v.Func, nil
		}
		// html/template 的函数调用参数以空格分隔
		return v.Func + " " + strings.Join(args, " "), nil

	case *Unary:
		x, err := render(v.X, s)
		if err != nil {
			return "", err
		}
		// html/template 的取反只能用 not 函数
		return "not " + maybeParen(v.X, x), nil

	case *Binary:
		l, err := render(v.L, s)
		if err != nil {
			return "", err
		}
		r, err := render(v.R, s)
		if err != nil {
			return "", err
		}
		if fn, ok := binaryFuncs[v.Op]; ok {
			return fn + " " + maybeParen(v.L, l) + " " + maybeParen(v.R, r), nil
		}
		// 算术运算符保留中缀写法
		return l + " " + v.Op + " " + r, nil

	default:
		return "", fmt.Errorf("vugo: 无法渲染的表达式节点 %T", e)
	}
}

// renderIdent 是变量转译的核心：决定一个标识渲染成 .field、$var 还是原样透传。
func renderIdent(v *Ident, s *Scope) string {
	// 已经是 Go 模板写法（$item / .Item）时不做改写
	if strings.HasPrefix(v.Name, "$") || strings.HasPrefix(v.Name, ".") {
		return joinPath(v.Name, v.Path)
	}
	if keyword, ok := keywords[v.Name]; ok && len(v.Path) == 0 {
		return keyword
	}

	prefix, ok := s.Resolve(v.Name)
	if !ok {
		// 根数据字段：msg -> .msg
		return joinPath("."+v.Name, v.Path)
	}
	if prefix == DotScope {
		// 点作用域：item -> . ，item.name -> .name
		if len(v.Path) == 0 {
			return "."
		}
		return "." + strings.Join(v.Path, ".")
	}
	return joinPath(prefix, v.Path)
}

func joinPath(head string, path []string) string {
	if len(path) == 0 {
		return head
	}
	return head + "." + strings.Join(path, ".")
}

// maybeParen 当子表达式是逻辑/比较运算时给它加括号，保持原有的运算优先级。
func maybeParen(e Expr, src string) string {
	switch e.(type) {
	case *Binary, *Unary:
		return "(" + src + ")"
	}
	return src
}
