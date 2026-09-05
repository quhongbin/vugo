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
