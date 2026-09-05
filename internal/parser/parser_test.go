package parser

import (
	"testing"

	"github.com/quhongbin/vugo/internal/ast"
)

func TestIsGoAction(t *testing.T) {
	cases := []struct {
		code string
		want bool
	}{
		{"msg", false},
		{"user.name", false},
		{"formatTime(time)", false},
		{".show", true},
		{"$item", true},
		{"if .show", true},
		{"else if .other", true},
		{"end", true},
		{"range .list", true},
		{`define "layout"`, true},
		{`template "footer" .`, true},
		{"len .items", true},
		{"printf \"%d\" .count", true},
		{"index .items 0", true},
		{"len(items)", false}, // 类 Vue 函数调用，交给 expr 转译
		{"index", false},      // 裸内置函数名优先当作用户变量（循环变量常叫 index）
	}
	for _, c := range cases {
		if got := isGoAction(c.code); got != c.want {
			t.Errorf("isGoAction(%q) = %v, 期望 %v", c.code, got, c.want)
		}
	}
}

func TestParseAttrs(t *testing.T) {
	attrs, err := parseAttrs(`id="a b" class='c' disabled data-x=1 v-for="(item, index) in list"`)
	if err != nil {
		t.Fatal(err)
	}
	want := []struct{ name, value string }{
		{"id", "a b"},
		{"class", "c"},
		{"disabled", ""},
		{"data-x", "1"},
		{"v-for", "(item, index) in list"},
	}
	if len(attrs) != len(want) {
		t.Fatalf("属性数量 = %d, 期望 %d: %+v", len(attrs), len(want), attrs)
	}
	for i, a := range attrs {
		if a.name != want[i].name || a.value != want[i].value {
			t.Errorf("属性[%d] = %+v, 期望 %+v", i, a, want[i])
		}
	}
}

func TestParseDirectiveClassification(t *testing.T) {
	doc, err := ParseString(`<a v-bind:href="url" v-on:click="go" @submit="s" :key="x.id" v-for="u in users"></a>`)
	if err != nil {
		t.Fatal(err)
	}
	el, ok := doc.Nodes[0].(*ast.Element)
	if !ok {
		t.Fatalf("根节点类型 = %T, 期望 *ast.Element", doc.Nodes[0])
	}

	if len(el.Directives) != 1 || el.Directives[0].Name != "for" || el.Directives[0].Value != "u in users" {
		t.Errorf("指令分类错误: %+v", el.Directives)
	}
	if len(el.Bindings) != 4 {
		t.Fatalf("绑定数量 = %d, 期望 4: %+v", len(el.Bindings), el.Bindings)
	}
	want := []ast.Binding{
		{Kind: ast.BindAttr, Attr: "href", Value: "url"},
		{Kind: ast.BindEvent, Attr: "click", Value: "go"},
		{Kind: ast.BindEvent, Attr: "submit", Value: "s"},
		{Kind: ast.BindAttr, Attr: "key", Value: "x.id"},
	}
	for i, b := range el.Bindings {
		if b.Kind != want[i].Kind || b.Attr != want[i].Attr || b.Value != want[i].Value {
			t.Errorf("绑定[%d] = %+v, 期望 %+v", i, b, want[i])
		}
	}
	if len(el.Attrs) != 0 {
		t.Errorf("不应有普通属性, 实际: %+v", el.Attrs)
	}
}
