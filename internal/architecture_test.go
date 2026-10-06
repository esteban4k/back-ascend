package internal_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const module = "ascend/internal/"

type rule struct {
	layer string
	// allowed internal layers this one may import
	allowed []string
	// forbidden standard library packages
	forbidden []string
}

var rules = []rule{
	{layer: "domain", allowed: []string{"domain"}, forbidden: []string{"net/http", "database/sql", "os", "encoding/json", "log", "log/slog"}},
	{layer: "application", allowed: []string{"domain", "application"}, forbidden: []string{"net/http", "database/sql", "os", "encoding/json", "log", "log/slog"}},
}

func TestLayersOnlyImportInward(t *testing.T) {
	for _, r := range rules {
		checked := 0
		err := filepath.WalkDir(r.layer, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			checked++
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
			if err != nil {
				return err
			}
			for _, spec := range file.Imports {
				imp, _ := strconv.Unquote(spec.Path.Value)
				if msg := violation(r, imp); msg != "" {
					t.Errorf("%s imports %q: %s", filepath.ToSlash(path), imp, msg)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if checked == 0 {
			t.Fatalf("no files found for layer %s", r.layer)
		}
	}
}

func violation(r rule, imp string) string {
	if rest, ok := strings.CutPrefix(imp, module); ok {
		target := strings.SplitN(rest, "/", 2)[0]
		for _, a := range r.allowed {
			if target == a {
				return ""
			}
		}
		return r.layer + " must not depend on " + target
	}
	if strings.Contains(strings.SplitN(imp, "/", 2)[0], ".") {
		return r.layer + " must not depend on third-party modules"
	}
	for _, f := range r.forbidden {
		if imp == f {
			return "I/O and delivery concerns belong in infrastructure"
		}
	}
	return ""
}
