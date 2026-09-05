// Command vugo 把类 Vue 语法的模板转译为 Go 标准库 html/template 模板。
//
// 用法：
//
//	vugo -src ./templates -dst ./dist        # 批量转译目录
//	vugo index.vugo > index.tmpl             # 转译单个文件到标准输出
//	cat index.vugo | vugo                    # 从标准输入读取
package main

import (
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/quhongbin/vugo/internal/compiler"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "vugo: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("vugo", flag.ContinueOnError)
	flags.SetOutput(stderr)

	src := flags.String("src", "", "输入目录（递归处理其中所有 -in 指定的文件），需配合 -dst 使用")
	dst := flags.String("dst", "", "输出目录")
	inExt := flags.String("in", ".vugo", "输入文件扩展名")
	outExt := flags.String("out", ".tmpl", "输出文件扩展名")
	skipValidate := flags.Bool("no-validate", false, "跳过 html/template 语法校验")
	funcNames := flags.String("funcs", "", "模板中用到的自定义函数名列表，逗号分隔；校验时会注册同名桩函数，如 formatTime,slugify")
	verbose := flags.Bool("v", false, "打印每个被处理的文件")

	flags.Usage = func() {
		fmt.Fprintf(stderr, "vugo：把类 Vue 模板转译为 html/template 模板\n\n用法:\n  vugo [选项] [文件...]\n\n选项:\n")
		flags.PrintDefaults()
		fmt.Fprintf(stderr, "\n示例:\n  vugo -src ./templates -dst ./dist\n  vugo index.vugo > index.tmpl\n  cat index.vugo | vugo\n")
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	c := compiler.New()
	c.Validate = !*skipValidate
	if f := stubFuncs(*funcNames); len(f) > 0 {
		c.Funcs = f
	}

	switch {
	case *src != "":
		if *dst == "" {
			return fmt.Errorf("-src 必须配合 -dst 使用")
		}
		n, err := compileDir(c, *src, *dst, *inExt, *outExt, *verbose, stderr)

		if err != nil {
			return err
		}
		fmt.Fprintf(stderr, "vugo: 已转译 %d 个文件到 %s\n", n, *dst)
		return nil

	case flags.NArg() > 0:
		for _, path := range flags.Args() {
			out, err := c.CompileFile(path)
			if err != nil {
				return err
			}
			if *dst == "" {
				fmt.Fprint(stdout, out)
				continue
			}
			dest := filepath.Join(*dst, replaceExt(filepath.Base(path), *inExt, *outExt))
			if err := os.MkdirAll(*dst, 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(dest, []byte(out), 0o644); err != nil {
				return err
			}
			report(*verbose, stderr, "%s -> %s\n", path, dest)
		}
		return nil

	default:
		// 无参数时从标准输入读取，结果写回标准输出
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("读取标准输入失败: %w", err)
		}
		out, err := c.Compile(string(data))
		if err != nil {
			return err
		}
		fmt.Fprint(stdout, out)
		return nil
	}
}

// compileDir 递归转译 src 目录下所有 inExt 文件，产物按相对路径写入 dst。
func compileDir(c *compiler.Compiler, src, dst, inExt, outExt string, verbose bool, stderr io.Writer) (int, error) {
	count := 0
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, inExt) {
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		out, err := c.CompileFile(path)
		if err != nil {
			return err
		}
		dest := filepath.Join(dst, replaceExt(rel, inExt, outExt))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dest, []byte(out), 0o644); err != nil {
			return err
		}
		count++
		report(verbose, stderr, "%s -> %s\n", path, dest)
		return nil
	})
	return count, err
}

// stubFuncs 为声明的函数名注册变参桩函数，仅用于通过 html/template 校验；
// Parse 只检查函数存在性，参数在运行时才求值，真实实现由业务侧注册。
func stubFuncs(names string) template.FuncMap {
	funcs := template.FuncMap{}
	for _, name := range strings.Split(names, ",") {
		if name = strings.TrimSpace(name); name != "" {
			funcs[name] = func(args ...any) any { return "" }
		}
	}
	return funcs
}

// replaceExt 把路径的扩展名 from 替换成 to。
func replaceExt(path, from, to string) string {
	if strings.HasSuffix(path, from) {
		path = path[:len(path)-len(from)]
	}
	return path + to
}

func report(verbose bool, w io.Writer, format string, args ...any) {
	if verbose {
		fmt.Fprintf(w, format, args...)
	}
}
