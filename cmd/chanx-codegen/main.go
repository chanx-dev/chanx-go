// Command chanx-codegen generates a typed Go client from a chanx AsyncAPI 3
// document, a file or a URL:
//
//	go run go.chanx.dev/cmd/chanx-codegen@latest -o ./chanxgen http://localhost:8000/asyncapi.json
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"go.chanx.dev/codegen"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "chanx-codegen:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("chanx-codegen", flag.ContinueOnError)
	out := fs.String("o", "chanxgen", "output directory")
	pkg := fs.String("package", "", "package name (default: the output directory's name)")
	fs.Usage = func() {
		_, _ = fmt.Fprintln(fs.Output(), "usage: chanx-codegen [-o dir] [-package name] <schema file or URL>")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return fmt.Errorf("want one schema, got %d", fs.NArg())
	}
	if *pkg == "" {
		*pkg = filepath.Base(filepath.Clean(*out))
	}
	doc, err := codegen.Load(context.Background(), fs.Arg(0))
	if err != nil {
		return err
	}
	files, err := codegen.Generate(doc, codegen.Options{Package: *pkg})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(*out, name), files[name], 0o644); err != nil {
			return err
		}
		fmt.Println("wrote", filepath.Join(*out, name))
	}
	return nil
}
