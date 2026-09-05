package transpiler

import (
	"fmt"
	"strings"

	"github.com/quhongbin/vugo/internal/expr"
)

// ForDirective 描述一个解析完成的 v-for 指令。
type ForDirective struct {
	Iterable string            // 可迭代对象的类 Vue 表达式，如 "user.todos"
	Frame    map[string]string // 循环变量 -> 模板前缀，进入循环体时压入作用域
	vars     []string          // [value] 或 [value, index]
}

// Header 生成 range 的开启 action（不含花括号）。
// iterable 为已转译后的可迭代对象表达式。
func (f *ForDirective) Header(iterable string) string {
	if len(f.vars) == 1 {
		// v-for="item in list" -> {{ range .list }}，循环体内用 . 访问元素
		return "range " + iterable
	}
	// v-for="(item, index) in list" -> {{ range $index, $item := .list }}
	return "range $" + f.vars[1] + ", $" + f.vars[0] + " := " + iterable
}

// parseFor 解析 v-for 指令的表达式。
//
// 支持两种形式：
//   - v-for="item in list"
//   - v-for="(item, index) in list"
func parseFor(value string) (*ForDirective, error) {
	split := splitIn(value)
	if split < 0 {
		return nil, fmt.Errorf("v-for 语法应为 \"item in list\" 或 \"(item, index) in list\"，实际为 %q", value)
	}

	left := strings.TrimSpace(value[:split])
	right := strings.TrimSpace(value[split+len(" in "):])
	if left == "" || right == "" {
		return nil, fmt.Errorf("v-for 语法应为 \"item in list\" 或 \"(item, index) in list\"，实际为 %q", value)
	}

	var names []string
	if strings.HasPrefix(left, "(") && strings.HasSuffix(left, ")") {
		for _, part := range strings.Split(left[1:len(left)-1], ",") {
			names = append(names, strings.TrimSpace(part))
		}
	} else {
		names = append(names, left)
	}

	d := &ForDirective{Iterable: right, vars: names, Frame: map[string]string{}}
	switch len(names) {
	case 1:
		// 单参数形式绑定到当前点作用域
	case 2:
		// Go 的 range 下标在前、元素在后，与 Vue 的书写顺序相反
	default:
		return nil, fmt.Errorf("v-for 暂不支持 %d 个循环变量（仅支持 1 个或 2 个）", len(names))
	}

	for _, name := range names {
		if !validVarName(name) {
			return nil, fmt.Errorf("v-for 的循环变量 %q 不是合法的标识符", name)
		}
		if _, dup := d.Frame[name]; dup {
			return nil, fmt.Errorf("v-for 的循环变量 %q 重复", name)
		}
	}
	for i, name := range names {
		if i == 0 && len(names) == 1 {
			d.Frame[name] = expr.DotScope
			continue
		}
		d.Frame[name] = "$" + name
	}
	return d, nil
}

// splitIn 返回顶层 " in " 的起始下标，找不到时返回 -1。
// 括号内的 " in "（如对象遍历）不会被误判。
func splitIn(value string) int {
	depth := 0
	for i := 0; i < len(value); i++ {
		switch value[i] {
		case '(':
			depth++
		case ')':
			depth--
		case ' ', '\t', '\n':
			if depth == 0 && strings.HasPrefix(value[i:], " in ") {
				return i
			}
		}
	}
	return -1
}

func validVarName(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' {
			continue
		}
		if i > 0 && c >= '0' && c <= '9' {
			continue
		}
		return false
	}
	return true
}
