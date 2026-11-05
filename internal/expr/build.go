package expr

import (
	"fmt"
	"strings"
	"text/template/parse"

	"github.com/quhongbin/vugo/internal/node"
)

// keywords 是类 Vue 表达式中的字面量关键字。
var keywords = map[string]bool{"true": true, "false": true, "null": true, "nil": true}

// binaryFuncs 把类 Vue 的中缀比较/逻辑运算符映射为 html/template 的内置函数。
// html/template 不支持 `==`、`&&` 这类中缀写法，只接受 eq、and 等函数形式。
var binaryFuncs = map[string]string{
	"==": "eq", "!=": "ne",
	">": "gt", ">=": "ge", "<": "lt", "<=": "le",
	"&&": "and", "||": "or",
}

// Build 把一个类 Vue 表达式直接构建为 text/template/parse 的管道节点。
//
// 例如 `a && !b` 会得到 PipeNode{ Command{ and, .a, (not .b) } }，
// 其 String() 为 `and .a (not .b)`。
//
// 参数:
//   - src:   表达式原文，如 `user.name`、`formatTime(time)`。
//   - base:  表达式首字节在 .vugo 源码中的偏移，节点 Pos = base + 表达式内偏移。
//   - scope: 当前作用域，决定变量构建为 `.field`、`.` 还是 `$var`；传 nil 按根作用域处理。
//   - f:     节点工厂，所有节点都由它创建。
//
// 返回值:
//   - *parse.PipeNode: 可直接放入 ActionNode / BranchNode 的管道。
//   - error: 语法错误或 html/template 无法表达的写法（算术运算、带点号的函数名、
//     未声明的 `$变量`、单独输出 nil）时返回，信息以 `vugo: 表达式` 开头。
func Build(src string, base int, scope *Scope, f *node.Factory) (*parse.PipeNode, error) {
	e, err := Parse(src)
	if err != nil {
		return nil, err
	}
	if scope == nil {
		scope = NewScope()
	}
	b := &builder{f: f, s: scope, base: base, src: src}
	cmd, err := b.command(e)
	if err != nil {
		return nil, err
	}
	return f.Pipe(base+e.pos(), nil, cmd), nil
}

// builder 把表达式 AST 转成 parse 节点。
type builder struct {
	f    *node.Factory
	s    *Scope
	base int
	src  string
}

func (b *builder) errorf(e Expr, format string, args ...any) error {
	return fmt.Errorf("vugo: 表达式 %q 第 %d 列: %s", b.src, e.pos()+1, fmt.Sprintf(format, args...))
}

// command 把表达式构建成管道中的一条命令。
func (b *builder) command(e Expr) (*parse.CommandNode, error) {
	pos := b.base + e.pos()
	switch v := e.(type) {
	case *Paren:
		return b.command(v.X)

	case *Call:
		if strings.Contains(v.Func, ".") {
			return nil, b.errorf(e, "函数名 %q 含点号，html/template 的函数名必须是合法标识符，请在 FuncMap 中以无点号的名字注册", v.Func)
		}
		args := []parse.Node{b.f.Identifier(pos, v.Func)}
		for _, a := range v.Args {
			op, err := b.operand(a)
			if err != nil {
				return nil, err
			}
			args = append(args, op)
		}
		return b.f.Command(pos, args...), nil

	case *Unary:
		x, err := b.operand(v.X)
		if err != nil {
			return nil, err
		}
		return b.f.Command(pos, b.f.Identifier(pos, "not"), x), nil

	case *Binary:
		fn, ok := binaryFuncs[v.Op]
		if !ok {
			return nil, b.errorf(e, "html/template 没有算术运算符 %q，请在 Go 后端预先计算，或注册自定义函数（如 add/sub）后改写为函数调用", v.Op)
		}
		l, err := b.operand(v.L)
		if err != nil {
			return nil, err
		}
		r, err := b.operand(v.R)
		if err != nil {
			return nil, err
		}
		return b.f.Command(pos, b.f.Identifier(pos, fn), l, r), nil

	default:
		op, err := b.operand(e)
		if err != nil {
			return nil, err
		}
		if _, isNil := op.(*parse.NilNode); isNil {
			return nil, b.errorf(e, "nil 不能单独作为输出，只能作为函数参数")
		}
		return b.f.Command(pos, op), nil
	}
}

// operand 把表达式构建成命令的一个参数；复合表达式会包成括号管道。
func (b *builder) operand(e Expr) (parse.Node, error) {
	pos := b.base + e.pos()
	switch v := e.(type) {
	case *Literal:
		if strings.HasPrefix(v.Value, `"`) {
			n, err := b.f.String(pos, v.Value)
			if err != nil {
				return nil, b.errorf(e, "%v", err)
			}
			return n, nil
		}
		n, err := b.f.Number(pos, v.Value)
		if err != nil {
			return nil, b.errorf(e, "%v", err)
		}
		return n, nil

	case *Ident:
		return b.ident(v)

	case *Paren:
		return b.operand(v.X)

	default:
		// 函数调用、一元、二元运算作为参数时必须加括号：(not .b)
		cmd, err := b.command(e)
		if err != nil {
			return nil, err
		}
		return b.f.Pipe(pos, nil, cmd), nil
	}
}

// ident 是变量构建的核心：决定一个标识构建成 FieldNode、DotNode、VariableNode 还是字面量。
func (b *builder) ident(v *Ident) (parse.Node, error) {
	pos := b.base + v.pos()
	// 已经是 Go 模板变量写法（$item）时直接使用，但必须已声明
	if strings.HasPrefix(v.Name, "$") {
		if !b.s.HasVar(v.Name) {
			return nil, b.errorf(v, "模板变量 %s 未声明", v.Name)
		}
		return b.f.Variable(pos, append([]string{v.Name}, v.Path...)...), nil
	}
	if keywords[v.Name] && len(v.Path) == 0 {
		switch v.Name {
		case "true":
			return b.f.Bool(pos, true), nil
		case "false":
			return b.f.Bool(pos, false), nil
		default:
			return b.f.Nil(pos), nil
		}
	}

	prefix, ok := b.s.Resolve(v.Name)
	switch {
	case !ok:
		// 根数据字段：msg -> .msg
		return b.f.Field(pos, append([]string{v.Name}, v.Path...)...), nil
	case prefix == DotScope:
		// 点作用域：item -> . ，item.name -> .name
		if len(v.Path) == 0 {
			return b.f.Dot(pos), nil
		}
		return b.f.Field(pos, v.Path...), nil
	default:
		return b.f.Variable(pos, append([]string{prefix}, v.Path...)...), nil
	}
}
