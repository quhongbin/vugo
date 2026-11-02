package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withStdin 临时把 os.Stdin 替换为一个包含 content 的文件，测试结束后自动还原。
//
// 参数:
//   - t:     测试句柄，用于创建临时目录与登记清理。
//   - content: 写入临时文件的内容，即"标准输入"要读到的字节。
//
// 副作用: 会修改全局变量 os.Stdin，测试结束后恢复原值；不可并行执行。
func withStdin(t *testing.T, content string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stdin")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdin
	os.Stdin = f
	t.Cleanup(func() {
		os.Stdin = orig
		f.Close()
	})
}

// writeFile 在 dir 下写入一个文件，测试用。
func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestRunStdinMode 覆盖无参数时的标准输入模式。
func TestRunStdinMode(t *testing.T) {
	withStdin(t, `<p>{{ msg }}</p>`)
	var stdout, stderr bytes.Buffer
	if err := run(nil, &stdout, &stderr); err != nil {
		t.Fatalf("run 失败: %v", err)
	}
	if got, want := stdout.String(), `<p>{{ .msg }}</p>`; got != want {
		t.Errorf("stdout = %q, 期望 %q", got, want)
	}
}

// TestRunFileMode 覆盖位置参数模式：无 -dst 时产物写标准输出。
func TestRunFileMode(t *testing.T) {
	src := writeFile(t, t.TempDir(), "index.vugo", `<h1>{{ user.name }}</h1>`)
	var stdout, stderr bytes.Buffer
	if err := run([]string{src}, &stdout, &stderr); err != nil {
		t.Fatalf("run 失败: %v", err)
	}
	if got, want := stdout.String(), `<h1>{{ .user.name }}</h1>`; got != want {
		t.Errorf("stdout = %q, 期望 %q", got, want)
	}
}

// TestRunFileModeToDir 覆盖位置参数 + -dst：产物按文件名写入目标目录。
func TestRunFileModeToDir(t *testing.T) {
	src := writeFile(t, t.TempDir(), "index.vugo", `<h1>{{ user.name }}</h1>`)
	dst := filepath.Join(t.TempDir(), "dist")
	var stdout, stderr bytes.Buffer
	if err := run([]string{"-dst", dst, src}, &stdout, &stderr); err != nil {
		t.Fatalf("run 失败: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dst, "index.tmpl"))
	if err != nil {
		t.Fatalf("产物未生成: %v", err)
	}
	if want := `<h1>{{ .user.name }}</h1>`; string(got) != want {
		t.Errorf("产物 = %q, 期望 %q", got, want)
	}
}

// TestRunDirMode 覆盖 -src/-dst 目录批处理：递归处理并保持相对目录结构。
func TestRunDirMode(t *testing.T) {
	srcDir := t.TempDir()
	writeFile(t, srcDir, "a.vugo", `<p>{{ a }}</p>`)
	writeFile(t, srcDir, "sub/b.vugo", `<p>{{ b }}</p>`)
	// 非 .vugo 后缀的文件应被跳过
	writeFile(t, srcDir, "skip.html", `<p>{{ c }}</p>`)

	dst := filepath.Join(t.TempDir(), "dist")
	var stdout, stderr bytes.Buffer
	if err := run([]string{"-src", srcDir, "-dst", dst, "-v"}, &stdout, &stderr); err != nil {
		t.Fatalf("run 失败: %v", err)
	}

	for name, want := range map[string]string{
		"a.tmpl":     `<p>{{ .a }}</p>`,
		"sub/b.tmpl": `<p>{{ .b }}</p>`,
	} {
		got, err := os.ReadFile(filepath.Join(dst, name))
		if err != nil {
			t.Errorf("产物 %s 未生成: %v", name, err)
			continue
		}
		if string(got) != want {
			t.Errorf("产物 %s = %q, 期望 %q", name, got, want)
		}
	}
	if _, err := os.Stat(filepath.Join(dst, "skip.tmpl")); !os.IsNotExist(err) {
		t.Errorf("非 .vugo 文件不应被转译")
	}
	if !strings.Contains(stderr.String(), "已转译 2 个文件") {
		t.Errorf("stderr 应报告处理数量, 实际: %q", stderr.String())
	}
}

// TestRunSrcWithoutDst -src 必须配合 -dst 使用。
func TestRunSrcWithoutDst(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run([]string{"-src", t.TempDir()}, &stdout, &stderr)
	if err == nil {
		t.Fatal("缺少 -dst 时应当报错")
	}
	if !strings.Contains(err.Error(), "-dst") {
		t.Errorf("错误信息应提示 -dst, 实际: %v", err)
	}
}

// TestRunUnsupportedSyntax 不支持的语法必须报错，且提示里带替代方案关键词。
func TestRunUnsupportedSyntax(t *testing.T) {
	src := writeFile(t, t.TempDir(), "bad.vugo", `<input v-model="name" />`)
	var stdout, stderr bytes.Buffer
	err := run([]string{src}, &stdout, &stderr)
	if err == nil {
		t.Fatal("v-model 应当报错")
	}
	if !strings.Contains(err.Error(), "v-model") {
		t.Errorf("错误信息应包含 v-model, 实际: %v", err)
	}
}

// TestRunFuncsStub -funcs 注册桩函数后，含自定义函数的模板才能通过校验。
func TestRunFuncsStub(t *testing.T) {
	src := writeFile(t, t.TempDir(), "f.vugo", `<p>{{ formatTime(time) }}</p>`)

	var stdout, stderr bytes.Buffer
	if err := run([]string{src}, &stdout, &stderr); err == nil {
		t.Error("未注册 formatTime 时校验应当失败")
	}

	stdout.Reset()
	stderr.Reset()
	if err := run([]string{"-funcs", "formatTime", src}, &stdout, &stderr); err != nil {
		t.Fatalf("注册桩函数后应当成功: %v", err)
	}
	if got, want := stdout.String(), `<p>{{ formatTime .time }}</p>`; got != want {
		t.Errorf("stdout = %q, 期望 %q", got, want)
	}
}

// TestRunNoValidate -no-validate 跳过 html/template 校验。
func TestRunNoValidate(t *testing.T) {
	src := writeFile(t, t.TempDir(), "f.vugo", `<p>{{ formatTime(time) }}</p>`)
	var stdout, stderr bytes.Buffer
	if err := run([]string{"-no-validate", src}, &stdout, &stderr); err != nil {
		t.Fatalf("跳过校验后应当成功: %v", err)
	}
	if got, want := stdout.String(), `<p>{{ formatTime .time }}</p>`; got != want {
		t.Errorf("stdout = %q, 期望 %q", got, want)
	}
}
