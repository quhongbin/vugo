package transpiler

import (
	"fmt"

	"github.com/quhongbin/vugo/internal/ast"
)

// unsupportedDirectives 是暂未实现的 v-* 指令及其替代方案。
// 按项目约定，遇到无法映射的语法必须明确告知用户并给出替代写法。
var unsupportedDirectives = map[string]string{
	"if":      "暂未实现，可直接使用原生 html/template：{{ if .cond }}...{{ end }}",
	"else":    "暂未实现，可直接使用原生 html/template：{{ else }}",
	"else-if": "暂未实现，可直接使用原生 html/template：{{ else if .cond }}",
	"show":    "暂未实现，可直接使用原生 html/template：{{ if .cond }}...{{ end }}",
	"model":   "服务端模板无双向绑定能力，请拆分为 :value 展示数据 + @input 提交到后端处理",
	"slot":    "请改用 {{ define \"SlotName\" }} + {{ template \"SlotName\" . }} 组合实现类似效果",
	"html":    "会绕过 html/template 的自动转义，本项目不予支持，请直接使用 {{ .field }}",
	"text":    "暂未实现，请直接使用 {{ .field }}",
	"once":    "服务端模板为一次性渲染，v-once 无意义，可直接使用 {{ .field }}",
	"pre":     "请改用原生 html/template 的 {{ .field }} 并移除该指令",
	"cloak":   "服务端模板无需 v-cloak，请移除该指令",
}

// resolveElement 检查元素上的指令与绑定是否都可转译，
// 并解析出 v-for 指令（不存在时返回 nil）。
func resolveElement(el *ast.Element) (*ForDirective, error) {
	var loop *ForDirective

	for _, d := range el.Directives {
		if d.Name != "for" {
			return nil, unsupportedDirective(d)
		}
		if loop != nil {
			return nil, fmt.Errorf("vugo: 第 %d 行: 同一个标签上只能使用一个 v-for", d.Line)
		}
		parsed, err := parseFor(d.Value)
		if err != nil {
			return nil, fmt.Errorf("vugo: 第 %d 行: %w", d.Line, err)
		}
		loop = parsed
	}

	for _, b := range el.Bindings {
		// :key 是 v-for 的配套提示，Go 模板没有对应语义，直接丢弃
		if b.Kind == ast.BindAttr && b.Attr == "key" {
			continue
		}
		return nil, unsupportedBinding(b)
	}

	return loop, nil
}

// unsupportedDirective 生成"指令不支持 + 替代方案"的错误。
func unsupportedDirective(d ast.Directive) error {
	tip, ok := unsupportedDirectives[d.Name]
	if !ok {
		tip = "暂未实现，请改用原生 html/template 语法"
	}
	return fmt.Errorf("vugo: 第 %d 行: 指令 v-%s %s", d.Line, d.Name, tip)
}

// unsupportedBinding 生成"属性/事件绑定不支持 + 替代方案"的错误。
func unsupportedBinding(b ast.Binding) error {
	if b.Kind == ast.BindEvent {
		return fmt.Errorf("vugo: 第 %d 行: 事件绑定 @%s 暂未实现，服务端模板无法处理客户端事件，请改用表单提交到后端处理",
			b.Line, b.Attr)
	}
	return fmt.Errorf("vugo: 第 %d 行: 属性绑定 :%s 暂未实现，请改用原生 html/template 写法，如 attr=\"{{ .field }}\"",
		b.Line, b.Attr)
}
