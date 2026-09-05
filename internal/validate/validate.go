// Package validate 是转译管道的第四层：校验转译产物能否被 html/template 正确解析。
//
// 转译器产出的模板必须能被标准库直接 Parse，否则说明改写逻辑有误。
package validate

import (
	"fmt"
	"html/template"
)

// Template 校验 src 是否符合 html/template 语法，返回 nil 表示通过。
// funcs 为模板中用到的自定义函数（与 html/template 的 Funcs 一致），
// 缺失函数会导致 Parse 失败，因此调用方需把运行时注册的函数一并提供。
func Template(name, src string, funcs template.FuncMap) error {
	t := template.New(name)
	if len(funcs) > 0 {
		t = t.Funcs(funcs)
	}
	if _, err := t.Parse(src); err != nil {
		return fmt.Errorf("vugo: 转译产物无法通过 html/template 解析: %w", err)
	}
	return nil
}
