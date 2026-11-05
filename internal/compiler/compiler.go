// Package compiler 是转译管道的编排层，把各层串成一条完整的流水线：
//
//	源码 --lexer--> token --parser--> vugo AST --transpiler--> parse.Tree --AddParseTree--> html/template.Template
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
	// Validate 为 true 时，校验产出的 parse.Tree 与函数定义（默认开启）。
	Validate bool
	// Name 是构建 parse.Tree 与校验时使用的模板名，会体现在错误信息中。
	Name string
	// Funcs 是模板中用到的自定义函数，校验及构建 Template 时会一并注册。
	Funcs template.FuncMap
}

// New 创建一个默认开启产物校验的编译器。
//
// 返回值:
//   - *Compiler: Validate=true、Name="vugo" 的编译器实例。
func New() *Compiler {
	return &Compiler{Validate: true, Name: "vugo"}
}

// CompileTree 把一份类 Vue 模板源码解析并构建为 text/template/parse 语法树集合（*transpiler.Result）。
//
// 参数:
//   - src: 类 Vue 模板源码字符串。
//
// 返回值:
//   - *transpiler.Result: 包含主模板 parse.Tree 与所有 define/block 子模板 parse.Tree 的结果，
//     可直接调用 result.AddTo(tmpl) 挂载到 html/template.Template。
//   - error: 词法、语法、转译或校验失败时返回非 nil 错误，信息以 `vugo:` 开头。
func (c *Compiler) CompileTree(src string) (*transpiler.Result, error) {
	doc, err := parser.ParseString(src)
	if err != nil {
		return nil, err
	}

	res, err := transpiler.New().Transpile(doc, c.name(), src)
	if err != nil {
		return nil, err
	}

	if c.Validate {
		if err := validate.Trees(c.name(), res.Trees(), c.Funcs); err != nil {
			return nil, err
		}
	}
	return res, nil
}

// CompileTemplate 把一份类 Vue 模板源码构建为 parse.Tree，并直接调用
// (*template.Template).AddParseTree 挂载到一个新建的 *html/template.Template 实例中返回。
//
// 参数:
//   - src: 类 Vue 模板源码字符串。
//
// 返回值:
//   - *template.Template: 已通过 AddParseTree 挂载主树及全部 define/block 子树的模板实例，可直接 Execute。
//   - error: 编译或挂载失败时返回非 nil 错误，信息以 `vugo:` 开头。
func (c *Compiler) CompileTemplate(src string) (*template.Template, error) {
	res, err := c.CompileTree(src)
	if err != nil {
		return nil, err
	}
	tmpl := template.New(c.name())
	if len(c.Funcs) > 0 {
		tmpl = tmpl.Funcs(c.Funcs)
	}
	return res.AddTo(tmpl)
}

// AddToTemplate 把一份类 Vue 模板源码构建为 parse.Tree，并调用
// (*template.Template).AddParseTree 挂载到调用方传入的 dst 模板实例中。
//
// 参数:
//   - dst: 目标 html/template.Template 实例，不能为 nil，且尚未执行过 Execute。
//   - src: 类 Vue 模板源码字符串。
//
// 返回值:
//   - *template.Template: 挂载主树后的模板实例。
//   - error: 编译或 AddParseTree 失败时返回非 nil 错误，信息以 `vugo:` 开头。
//
// 副作用: 会向 dst 的命名空间中注册主模板及所有 define/block 子模板。
func (c *Compiler) AddToTemplate(dst *template.Template, src string) (*template.Template, error) {
	res, err := c.CompileTree(src)
	if err != nil {
		return nil, err
	}
	return res.AddTo(dst)
}

// Compile 把一份类 Vue 模板源码先构建为 parse.Tree 语法树，再序列化为等价的 html/template 源码。
//
// 参数:
//   - src: 类 Vue 模板源码字符串。
//
// 返回值:
//   - string: 由 parse.Tree 还原出的标准 html/template 模板源码。
//   - error:  编译或校验失败时返回非 nil 错误，信息以 `vugo:` 开头。
func (c *Compiler) Compile(src string) (string, error) {
	res, err := c.CompileTree(src)
	if err != nil {
		return "", err
	}
	out := res.String()
	if c.Validate {
		if err := validate.Template(c.name(), out, c.Funcs); err != nil {
			return "", err
		}
	}
	return out, nil
}

// CompileFile 读取并转译一个模板文件，返回产物源码。
//
// 参数:
//   - path: 模板文件路径。
//
// 返回值:
//   - string: 转译后的 html/template 模板源码。
//   - error:  读取文件或转译失败时返回非 nil 错误，错误信息会带上文件路径前缀。
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
//
// 参数:
//   - src: 类 Vue 模板源码字符串。
//
// 返回值:
//   - string: 转译后的 html/template 模板源码。
//   - error:  转译失败时返回非 nil 错误。
func Transpile(src string) (string, error) {
	return New().Compile(src)
}

// TranspileTree 是一次性构建语法树的便捷入口，等价于 New().CompileTree(src)。
//
// 参数:
//   - src: 类 Vue 模板源码字符串。
//
// 返回值:
//   - *transpiler.Result: 包含主模板与子模板 parse.Tree 的结果。
//   - error: 转译失败时返回非 nil 错误。
func TranspileTree(src string) (*transpiler.Result, error) {
	return New().CompileTree(src)
}

func (c *Compiler) name() string {
	if c.Name == "" {
		return "vugo"
	}
	return c.Name
}
