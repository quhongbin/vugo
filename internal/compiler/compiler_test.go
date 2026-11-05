package compiler_test

import (
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/quhongbin/vugo/internal/compiler"
)

// TestCompile 覆盖最基本的一次转译。
func TestCompile(t *testing.T) {
	c := compiler.New()
	got, err := c.Compile(`<p>{{ msg }}</p>`)
	if err != nil {
		t.Fatalf("Compile 失败: %v", err)
	}
	if want := `<p>{{ .msg }}</p>`; got != want {
		t.Errorf("Compile = %q, 期望 %q", got, want)
	}
}

// TestTranspile 覆盖一次性便捷入口。
func TestTranspile(t *testing.T) {
	got, err := compiler.Transpile(`<ul><li v-for="i in items">{{ i.name }}</li></ul>`)
	if err != nil {
		t.Fatalf("Transpile 失败: %v", err)
	}
	if want := `<ul>{{ range .items }}<li>{{ .name }}</li>{{ end }}</ul>`; got != want {
		t.Errorf("Transpile = %q, 期望 %q", got, want)
	}
}

// TestCompileValidateToggle 验证 Validate 开关：关闭后允许产出无法通过校验的模板。
func TestCompileValidateToggle(t *testing.T) {
	const src = `<p>{{ formatTime(time) }}</p>`

	strict := compiler.New()
	if _, err := strict.Compile(src); err == nil {
		t.Error("未注册 formatTime 时校验应当失败")
	}

	loose := compiler.New()
	loose.Validate = false
	got, err := loose.Compile(src)
	if err != nil {
		t.Fatalf("关闭校验后应当成功: %v", err)
	}
	if want := `<p>{{ formatTime .time }}</p>`; got != want {
		t.Errorf("Compile = %q, 期望 %q", got, want)
	}
}

// TestCompileWithFuncs 验证注入 Funcs 后自定义函数可以通过校验。
func TestCompileWithFuncs(t *testing.T) {
	c := compiler.New()
	c.Funcs = template.FuncMap{"formatTime": func(v any) string { return "" }}
	if _, err := c.Compile(`<p>{{ formatTime(time) }}</p>`); err != nil {
		t.Fatalf("注册 Funcs 后应当成功: %v", err)
	}
}

// TestCompileFile 覆盖文件入口，并确认失败时错误里带上了文件路径。
func TestCompileFile(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.vugo")
	if err := os.WriteFile(good, []byte(`<p>{{ msg }}</p>`), 0o644); err != nil {
		t.Fatal(err)
	}
	c := compiler.New()
	got, err := c.CompileFile(good)
	if err != nil {
		t.Fatalf("CompileFile 失败: %v", err)
	}
	if want := `<p>{{ .msg }}</p>`; got != want {
		t.Errorf("CompileFile = %q, 期望 %q", got, want)
	}

	bad := filepath.Join(dir, "bad.vugo")
	if err := os.WriteFile(bad, []byte(`<input v-model="x" />`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = c.CompileFile(bad)
	if err == nil {
		t.Fatal("不支持的语法应当报错")
	}
	if !strings.Contains(err.Error(), bad) {
		t.Errorf("错误信息应包含文件路径 %q, 实际: %v", bad, err)
	}
}

// TestCompileFileNotFound 读取不存在的文件应返回错误。
func TestCompileFileNotFound(t *testing.T) {
	c := compiler.New()
	if _, err := c.CompileFile(filepath.Join(t.TempDir(), "missing.vugo")); err == nil {
		t.Fatal("文件不存在时应当报错")
	}
}

// TestCompileErrorPrefix 确认错误信息以 "vugo:" 开头并带行号，便于调用方做错误匹配。
func TestCompileErrorPrefix(t *testing.T) {
	c := compiler.New()
	_, err := c.Compile("<div>\n  <p v-if=\"show\">x</p>\n</div>")
	if err == nil {
		t.Fatal("v-if 应当报错")
	}
	if !strings.HasPrefix(err.Error(), "vugo:") {
		t.Errorf("错误信息应以 vugo: 开头, 实际: %v", err)
	}
	if !strings.Contains(err.Error(), "第 2 行") {
		t.Errorf("错误信息应包含行号, 实际: %v", err)
	}
}

// TestCompileTemplateAndAddParseTree 验证直接构建 parse.Tree 并通过 AddParseTree 挂载到 html/template 执行。
func TestCompileTemplateAndAddParseTree(t *testing.T) {
	src := `{{ define "badge" }}<span>[{{ label }}]</span>{{ end }}` +
		`<div>{{ template "badge" . }}<ul><li v-for="(item, idx) in items">{{ idx }}:{{ item.name }}</li></ul></div>`

	c := compiler.New()
	tmpl, err := c.CompileTemplate(src)
	if err != nil {
		t.Fatalf("CompileTemplate 失败: %v", err)
	}

	var sb strings.Builder
	data := map[string]any{
		"label": "<admin>",
		"items": []map[string]any{
			{"name": "<b>alice</b>"},
			{"name": "bob"},
		},
	}
	if err := tmpl.Execute(&sb, data); err != nil {
		t.Fatalf("Execute 失败: %v", err)
	}
	want := `<div><span>[&lt;admin&gt;]</span><ul><li>0:&lt;b&gt;alice&lt;/b&gt;</li><li>1:bob</li></ul></div>`
	if got := sb.String(); got != want {
		t.Errorf("Execute = %q, 期望 %q", got, want)
	}
}

// TestAddToTemplateEscapeErrorLocation 验证当 html/template 在转义阶段报错时，
// 节点的 Pos 与 tr 能正确指向原始 .vugo 源码的行号。
func TestAddToTemplateEscapeErrorLocation(t *testing.T) {
	// 第 3 行在 <script> 的字符串字面量中未闭合引号便结束 <script>，触发 html/template 转义错误
	src := "<div>\n  <p>{{ msg }}</p>\n  <script>var x = \"{{ bad }};</script>\n</div>"
	c := compiler.New()
	c.Name = "page.vugo"
	tmpl, err := c.CompileTemplate(src)
	if err != nil {
		t.Fatalf("CompileTemplate 失败: %v", err)
	}
	var sb strings.Builder
	err = tmpl.Execute(&sb, map[string]any{"msg": "hi", "bad": "1"})
	if err == nil {
		t.Fatal("未闭合的 JS 字符串应当在 escape 阶段报错")
	}
	if !strings.Contains(err.Error(), "html/template:page.vugo:") {
		t.Errorf("期望报错包含模板名 page.vugo, 实际: %v", err)
	}
}
