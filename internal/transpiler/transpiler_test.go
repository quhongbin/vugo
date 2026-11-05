package transpiler_test

import (
	"bytes"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/quhongbin/vugo/internal/compiler"
)

// funcs 是测试模板中用到的自定义函数桩。
var funcs = template.FuncMap{
	"formatTime": func(v any) string { return "" },
}

func mustTranspile(t *testing.T, src string) string {
	t.Helper()
	c := compiler.New()
	c.Funcs = funcs
	out, err := c.Compile(src)
	if err != nil {
		t.Fatalf("转译失败: %v", err)
	}
	return out
}

// TestGoldenFiles 遍历 testdata 下的 *.vugo，与同名 *.tmpl 黄金文件逐字节对照。
func TestGoldenFiles(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "..", "testdata", "*.vugo"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("testdata 下没有找到 .vugo 用例")
	}
	for _, srcPath := range paths {
		srcPath := srcPath
		t.Run(filepath.Base(srcPath), func(t *testing.T) {
			src, err := os.ReadFile(srcPath)
			if err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile(strings.TrimSuffix(srcPath, ".vugo") + ".tmpl")
			if err != nil {
				t.Fatal(err)
			}
			got := mustTranspile(t, string(src))
			if got != string(want) {
				t.Errorf("转译结果与黄金文件不一致\n--- got ---\n%s\n--- want ---\n%s", got, want)
			}
		})
	}
}

// TestForExecution 验证 v-for 转译后的语义（不只是语法）。
func TestForExecution(t *testing.T) {
	out := mustTranspile(t, `<ul><li v-for="(item, i) in items">{{ i }}: {{ item.name }}</li></ul>`)
	tmpl, err := template.New("t").Parse(out)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	data := map[string]any{
		"items": []map[string]any{{"name": "a"}, {"name": "b"}},
	}
	if err := tmpl.Execute(&buf, data); err != nil {
		t.Fatal(err)
	}
	want := "<ul><li>0: a</li><li>1: b</li></ul>"
	if got := buf.String(); got != want {
		t.Errorf("渲染结果 = %q, 期望 %q", got, want)
	}
}

// TestNestedForExecution 验证嵌套 v-for 的作用域语义：
// 外层单变量循环把元素绑定到点作用域，内层循环的可迭代对象要在外层点作用域中求值。
func TestNestedForExecution(t *testing.T) {
	out := mustTranspile(t, `<div><section v-for="group in groups"><h2>{{ group.title }}</h2>`+
		`<ul><li v-for="(child, idx) in group.children">{{ idx }}. {{ child.name }}</li></ul>`+
		`</section></div>`)
	tmpl, err := template.New("t").Parse(out)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	data := map[string]any{
		"groups": []map[string]any{
			{"title": "A", "children": []map[string]any{{"name": "a1"}, {"name": "a2"}}},
			{"title": "B", "children": []map[string]any{{"name": "b1"}}},
		},
	}
	if err := tmpl.Execute(&buf, data); err != nil {
		t.Fatal(err)
	}
	want := "<div><section><h2>A</h2><ul><li>0. a1</li><li>1. a2</li></ul></section>" +
		"<section><h2>B</h2><ul><li>0. b1</li></ul></section></div>"
	if got := buf.String(); got != want {
		t.Errorf("渲染结果 = %q, 期望 %q", got, want)
	}
}

// TestXSSAutoEscape 确保转译没有破坏 html/template 的自动转义。
func TestXSSAutoEscape(t *testing.T) {
	out := mustTranspile(t, `<p>{{ comment }}</p>`)
	tmpl, err := template.New("t").Parse(out)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, map[string]any{"comment": "<script>alert(1)</script>"}); err != nil {
		t.Fatal(err)
	}
	want := "<p>&lt;script&gt;alert(1)&lt;/script&gt;</p>"
	if got := buf.String(); got != want {
		t.Errorf("用户输入未被自动转义: %q", got)
	}
}

// TestUnsupportedSyntax 确认不支持语法的报错包含替代方案提示。
func TestUnsupportedSyntax(t *testing.T) {
	cases := []struct{ name, src, wantKeyword string }{
		{"v-if", `<div v-if="show">x</div>`, "v-if"},
		{"v-model", `<input v-model="name" />`, "v-model"},
		{"v-slot", `<template v-slot:header>x</template>`, "v-slot"},
		{"attr-bind", `<a :href="url">x</a>`, ":href"},
		{"event-bind", `<button @click="go">x</button>`, "@click"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			comp := compiler.New()
			comp.Validate = false
			_, err := comp.Compile(c.src)
			if err == nil {
				t.Fatalf("%s 应当报错", c.name)
			}
			if !strings.Contains(err.Error(), c.wantKeyword) {
				t.Errorf("错误信息应包含 %q, 实际: %v", c.wantKeyword, err)
			}
		})
	}
}

// TestDuplicateVFor 同一标签上的多个 v-for 应当报错。
func TestDuplicateVFor(t *testing.T) {
	comp := compiler.New()
	comp.Validate = false
	_, err := comp.Compile(`<li v-for="a in b" v-for="c in d">x</li>`)
	if err == nil || !strings.Contains(err.Error(), "v-for") {
		t.Errorf("重复 v-for 应当报错, 实际: %v", err)
	}
}

// TestCrossElementControlStructureError 验证原生控制结构跨越 HTML 标签闭合边界时明确报错。
func TestCrossElementControlStructureError(t *testing.T) {
	comp := compiler.New()
	_, err := comp.Compile(`<div>{{ if .wrap }}<p>x</div>{{ end }}</p>`)
	if err == nil {
		t.Fatal("跨标签边界的 {{ if }}...{{ end }} 在 AST 模式下应当报错")
	}
	if !strings.Contains(err.Error(), "跨越") && !strings.Contains(err.Error(), "闭合") {
		t.Errorf("错误信息应说明跨标签闭合限制, 实际: %v", err)
	}
}

// TestBareMustacheInTagAttrListError 验证标签属性列表中裸写的 {{ if ... }} 报错。
func TestBareMustacheInTagAttrListError(t *testing.T) {
	comp := compiler.New()
	_, err := comp.Compile(`<input {{ if .disabled }}disabled{{ end }} />`)
	if err == nil {
		t.Fatal("属性列表中直接出现 {{ }} 在 AST 模式下应当报错")
	}
}

// TestInterpolationInAttrValue 验证属性值引号内的 {{ expr }} 可以正常构建为 parse.Tree 并转义执行。
func TestInterpolationInAttrValue(t *testing.T) {
	tmpl, err := compiler.New().CompileTemplate(`<a href="/u/{{ user.id }}" title="{{ user.name }}">link</a>`)
	if err != nil {
		t.Fatalf("CompileTemplate 失败: %v", err)
	}
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, map[string]any{
		"user": map[string]any{"id": "42", "name": `a"b`},
	})
	if err != nil {
		t.Fatalf("Execute 失败: %v", err)
	}
	want := `<a href="/u/42" title="a&#34;b">link</a>`
	if got := buf.String(); got != want {
		t.Errorf("渲染结果 = %q, 期望 %q", got, want)
	}
}
