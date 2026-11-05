package expr

import (
	"html/template"
	"strings"
	"testing"

	"github.com/quhongbin/vugo/internal/node"
)

func buildPipeString(src string, scope *Scope) (string, error) {
	f, err := node.New("test", src)
	if err != nil {
		return "", err
	}
	pipe, err := Build(src, 0, scope, f)
	if err != nil {
		return "", err
	}
	return pipe.String(), nil
}

func TestBuildRootScope(t *testing.T) {
	s := NewScope()
	s.Declare("$item")

	cases := []struct{ src, want string }{
		{"msg", ".msg"},
		{"user.name", ".user.name"},
		{"user.profile.email", ".user.profile.email"},
		{"formatTime(time)", "formatTime .time"},
		{"formatTime(user.createdAt)", "formatTime .user.createdAt"},
		{"a == b", "eq .a .b"},
		{"a != b", "ne .a .b"},
		{"a >= b", "ge .a .b"},
		{`'hello'`, `"hello"`},
		{"42", "42"},
		{"true", "true"},
		{"fn(null)", "fn nil"},
		{"$item", "$item"}, // 已声明的 Go 模板变量原样生成 VariableNode
		{"$", "$"},         // 根变量 $ 始终可见
		{"!visible", "not .visible"},
		{"a && b", "and .a .b"},
		{"a || b && c", "or .a (and .b .c)"},
		{"toUpper(name)", "toUpper .name"},
	}
	for _, c := range cases {
		got, err := buildPipeString(c.src, s)
		if err != nil {
			t.Errorf("Build(%q) 报错: %v", c.src, err)
			continue
		}
		if got != c.want {
			t.Errorf("Build(%q) = %q, 期望 %q", c.src, got, c.want)
		}
	}
}

func TestBuildWithLoopScope(t *testing.T) {
	// 模拟两层嵌套 v-for
	s := NewScope()
	s.Push(map[string]string{"item": DotScope}) // v-for="item in list"
	s.Push(map[string]string{"user": "$user", "i": "$i"})

	cases := []struct{ src, want string }{
		{"item", "."},          // 点作用域
		{"item.name", ".name"}, // 点作用域取字段
		{"user.name", "$user.name"},
		{"i", "$i"},
		{"greet(user)", "greet $user"},
		{"other", ".other"}, // 未绑定的变量回落到根数据
	}
	for _, c := range cases {
		got, err := buildPipeString(c.src, s)
		if err != nil {
			t.Errorf("Build(%q) 报错: %v", c.src, err)
			continue
		}
		if got != c.want {
			t.Errorf("Build(%q) = %q, 期望 %q", c.src, got, c.want)
		}
	}

	s.Pop()
	if got, _ := buildPipeString("user.name", s); got != ".user.name" {
		t.Errorf("退出循环作用域后 user.name = %q, 期望 .user.name", got)
	}
}

func TestBuildErrors(t *testing.T) {
	cases := []string{
		"",                   // 空表达式
		"user.",              // 点号后缺字段
		"a +",                // 缺右操作数
		"(a + b",             // 缺右括号
		"item[0]",            // 下标语法暂不支持
		"a ? b : c",          // 三元运算符不支持
		"count + 1",          // html/template AST 不支持算术运算符
		"item.price * 2",     // html/template AST 不支持算术运算符
		"strings.ToUpper(x)", // html/template 函数名不能包含点号
		"$undeclared",        // 未声明的模板变量
		"null",               // nil 不能单独作为命令输出
	}
	for _, src := range cases {
		if _, err := buildPipeString(src, nil); err == nil {
			t.Errorf("Build(%q) 期望报错，实际成功", src)
		}
	}
}

func TestBooleanFunctionsAreValidGoTemplate(t *testing.T) {
	// 确保构建出的 PipeNode 放入 ActionNode 后能通过 AddParseTree 被 html/template 执行
	const exprSrc = "a && !b"
	f, err := node.New("t", exprSrc)
	if err != nil {
		t.Fatal(err)
	}
	pipe, err := Build(exprSrc, 0, nil, f)
	if err != nil {
		t.Fatal(err)
	}
	if got := pipe.String(); got != "and .a (not .b)" {
		t.Fatalf("Pipe.String() = %q, 期望 %q", got, "and .a (not .b)")
	}

	root := f.List(0)
	root.Nodes = append(root.Nodes, f.Action(0, pipe))
	tree := f.MainTree(root)

	tmpl, err := template.New("t").AddParseTree("t", tree)
	if err != nil {
		t.Fatalf("AddParseTree 失败: %v", err)
	}
	var sb strings.Builder
	if err := tmpl.Execute(&sb, map[string]any{"a": true, "b": false}); err != nil {
		t.Fatal(err)
	}
	if got := sb.String(); got != "true" {
		t.Errorf("执行结果 = %q, 期望 %q", got, "true")
	}
}
