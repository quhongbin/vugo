package expr

import (
	"html/template"
	"strings"
	"testing"
)

func TestRenderRootScope(t *testing.T) {
	cases := []struct{ src, want string }{
		{"msg", ".msg"},
		{"user.name", ".user.name"},
		{"user.profile.email", ".user.profile.email"},
		{"formatTime(time)", "formatTime .time"},
		{"formatTime(user.createdAt)", "formatTime .user.createdAt"},
		{"count + 1", ".count + 1"},
		{"a == b", "eq .a .b"},
		{"a != b", "ne .a .b"},
		{"a >= b", "ge .a .b"},
		{"item.price * 2", ".item.price * 2"},
		{`'hello'`, `"hello"`},
		{"42", "42"},
		{"true", "true"},
		{"null", "nil"},
		{"$item", "$item"}, // Go 模板变量原样透传
		{"!visible", "not .visible"},
		{"a && b", "and .a .b"},
		{"a || b && c", "or .a (and .b .c)"},
		{"(a + b) * 2", "(.a + .b) * 2"},
		{"strings.ToUpper(name)", "strings.ToUpper .name"},
	}
	for _, c := range cases {
		got, err := Render(c.src, nil)
		if err != nil {
			t.Errorf("Render(%q) 报错: %v", c.src, err)
			continue
		}
		if got != c.want {
			t.Errorf("Render(%q) = %q, 期望 %q", c.src, got, c.want)
		}
	}
}

func TestRenderWithLoopScope(t *testing.T) {
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
		got, err := Render(c.src, s)
		if err != nil {
			t.Errorf("Render(%q) 报错: %v", c.src, err)
			continue
		}
		if got != c.want {
			t.Errorf("Render(%q) = %q, 期望 %q", c.src, got, c.want)
		}
	}

	s.Pop()
	if got, _ := Render("user.name", s); got != ".user.name" {
		t.Errorf("退出循环作用域后 user.name = %q, 期望 .user.name", got)
	}
}

func TestRenderErrors(t *testing.T) {
	cases := []string{
		"",          // 空表达式
		"user.",     // 点号后缺字段
		"a +",       // 缺右操作数
		"(a + b",    // 缺右括号
		"item[0]",   // 下标语法暂不支持
		"a ? b : c", // 三元运算符不支持
	}
	for _, src := range cases {
		if _, err := Render(src, nil); err == nil {
			t.Errorf("Render(%q) 期望报错，实际成功", src)
		}
	}
}

func TestBooleanFunctionsAreValidGoTemplate(t *testing.T) {
	// 确保映射出的 and/not 函数写法能被 html/template 接受并正确求值
	src, err := Render("a && !b", nil)
	if err != nil {
		t.Fatal(err)
	}
	if src != "and .a (not .b)" {
		t.Fatalf("Render = %q, 期望 %q", src, "and .a (not .b)")
	}
	tmpl, err := template.New("t").Parse("{{ " + src + " }}")
	if err != nil {
		t.Fatalf("解析 %q 失败: %v", src, err)
	}
	var sb strings.Builder
	if err := tmpl.Execute(&sb, map[string]any{"a": true, "b": false}); err != nil {
		t.Fatal(err)
	}
	if got := sb.String(); got != "true" {
		t.Errorf("执行结果 = %q, 期望 %q", got, "true")
	}
}
