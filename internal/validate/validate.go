// Package validate 是转译管道的第四层：校验转译产出的语法树及模板源码能否被 html/template 正确接收与解析。
//
// 本层支持两种校验：
//  1. Trees：把转译器生成的 []*parse.Tree 直接通过 (*template.Template).AddParseTree 挂载到
//     html/template.Template 上，并检查其中的自定义函数是否均已定义；
//  2. Template：用 html/template.Parse 校验序列化后的源码。
package validate

import (
	"fmt"
	"html/template"
	"text/template/parse"
)

// builtins 是 html/template 内置的全局函数名集合。
var builtins = map[string]bool{
	"and": true, "call": true, "html": true, "index": true, "slice": true, "js": true,
	"len": true, "not": true, "or": true, "print": true, "printf": true, "println": true,
	"urlquery": true, "eq": true, "ne": true, "lt": true, "le": true, "gt": true, "ge": true,
}

// Trees 校验一组 parse.Tree 能否通过 (*template.Template).AddParseTree 挂载到 html/template，
// 并遍历语法树检查所有函数调用（IdentifierNode）是否均在内置函数或 funcs 中定义。
//
// 参数:
//   - name:  主模板名称。
//   - trees: 待校验的语法树列表（通常来自 transpiler.Result.Trees()）。
//   - funcs: 模板中用到的自定义函数表（与 html/template 的 Funcs 一致）。
//
// 返回值:
//   - error: 函数未定义或 AddParseTree 失败时返回非 nil 错误，信息以 `vugo:` 开头。
//
// 副作用: 不会修改入参 trees 中的任何节点（挂载时使用 Copy 副本）。
func Trees(name string, trees []*parse.Tree, funcs template.FuncMap) error {
	t := template.New(name)
	if len(funcs) > 0 {
		t = t.Funcs(funcs)
	}
	for _, tr := range trees {
		if tr == nil || tr.Root == nil {
			continue
		}
		if err := checkFuncs(tr.Root, funcs); err != nil {
			return err
		}
		if _, err := t.AddParseTree(tr.Name, tr.Copy()); err != nil {
			return fmt.Errorf("vugo: 语法树 %q 无法通过 html/template.AddParseTree 挂载: %w", tr.Name, err)
		}
	}
	return nil
}

// Template 校验模板源码 src 是否符合 html/template 语法，返回 nil 表示通过。
//
// 参数:
//   - name:  模板名称，会出现在 html/template.Parse 的错误信息中。
//   - src:   待校验的模板源码。
//   - funcs: 模板中用到的自定义函数（与 html/template 的 Funcs 一致），
//     缺失函数会导致 Parse 失败，因此调用方需把运行时注册的函数一并提供。
//
// 返回值:
//   - error: Parse 失败时返回非 nil 错误，信息以 `vugo: 转译产物无法通过 html/template 解析:` 开头。
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

// checkFuncs 递归检查节点中调用的函数是否已定义。
func checkFuncs(n parse.Node, funcs template.FuncMap) error {
	if n == nil {
		return nil
	}
	switch v := n.(type) {
	case *parse.ListNode:
		for _, c := range v.Nodes {
			if err := checkFuncs(c, funcs); err != nil {
				return err
			}
		}
	case *parse.ActionNode:
		return checkFuncs(v.Pipe, funcs)
	case *parse.IfNode:
		return checkBranch(&v.BranchNode, funcs)
	case *parse.RangeNode:
		return checkBranch(&v.BranchNode, funcs)
	case *parse.WithNode:
		return checkBranch(&v.BranchNode, funcs)
	case *parse.TemplateNode:
		if v.Pipe != nil {
			return checkFuncs(v.Pipe, funcs)
		}
	case *parse.PipeNode:
		for _, cmd := range v.Cmds {
			if err := checkFuncs(cmd, funcs); err != nil {
				return err
			}
		}
	case *parse.CommandNode:
		for _, arg := range v.Args {
			if err := checkFuncs(arg, funcs); err != nil {
				return err
			}
		}
	case *parse.ChainNode:
		return checkFuncs(v.Node, funcs)
	case *parse.IdentifierNode:
		if !builtins[v.Ident] && (funcs == nil || funcs[v.Ident] == nil) {
			return fmt.Errorf("vugo: 转译产物无法通过 html/template 解析: template: function %q not defined", v.Ident)
		}
	}
	return nil
}

func checkBranch(b *parse.BranchNode, funcs template.FuncMap) error {
	if err := checkFuncs(b.Pipe, funcs); err != nil {
		return err
	}
	if err := checkFuncs(b.List, funcs); err != nil {
		return err
	}
	if b.ElseList != nil {
		return checkFuncs(b.ElseList, funcs)
	}
	return nil
}
