// Package compiler 是转译管道的编排层，把各层串成一条完整的流水线：
//
//	源码 --lexer--> token --parser--> AST --transpiler--> 模板 --validate--> 产物
//
// 各层之间只依赖下一层的公开接口，便于单独替换与测试。
package compiler

import (
	"fmt"
	"html/template"
	"os"

	"github.com/quhongbin/vugo/internal/parser"
	"github.com/quhongbin/vugo/internal/transpiler"
	"github.com/quhongbin/vugo/internal/validate"
)

// Compiler 编排一次转译过程。
type Compiler struct {
	// Validate 为 true 时，用 html/template.Parse 校验产物（默认开启）。
	Validate bool
	// Name 是校验时使用的模板名，会体现在 Parse 的错误信息中。
	Name string
	// Funcs 是模板中用到的自定义函数，校验时会一并注册。
	Funcs template.FuncMap
}

// New 创建一个默认开启产物校验的编译器。
func New() *Compiler {
	return &Compiler{Validate: true, Name: "vugo"}
}

// Compile 把一份类 Vue 模板源码转译为 html/template 源码。
func (c *Compiler) Compile(src string) (string, error) {
	doc, err := parser.ParseString(src)
	if err != nil {
		return "", err
	}

	out, err := transpiler.New().Transpile(doc)
	if err != nil {
		return "", err
	}

	if c.Validate {
		if err := validate.Template(c.name(), out, c.Funcs); err != nil {
			return "", err
		}
	}
	return out, nil
}

// CompileFile 读取并转译一个模板文件，返回产物源码。
// 错误信息会带上文件路径，便于定位。
func (c *Compiler) CompileFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("vugo: 读取模板失败: %w", err)
	}
	out, err := c.Compile(string(data))
	if err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	return out, nil
}

// Transpile 是一次性调用的便捷入口，等价于 New().Compile(src)。
func Transpile(src string) (string, error) {
	return New().Compile(src)
}

func (c *Compiler) name() string {
	if c.Name == "" {
		return "vugo"
	}
	return c.Name
}
