// Package codeindex builds a bounded, read-only snapshot of explicitly configured
// Go repositories. It is not a language server or a resolved call graph.
package codeindex

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Repository struct {
	Name    string `json:"name"`
	Project string `json:"project"`
	Path    string `json:"path"`
}

type Symbol struct {
	Repository string   `json:"repository"`
	Project    string   `json:"project"`
	Path       string   `json:"path"`
	Name       string   `json:"name"`
	Kind       string   `json:"kind"`
	Line       int      `json:"line"`
	EndLine    int      `json:"end_line"`
	Calls      []string `json:"calls"`
	Reference  string   `json:"reference"`
}

type Index struct {
	Symbols   []Symbol  `json:"symbols"`
	IndexedAt time.Time `json:"indexed_at"`
}

const maxFiles = 2000
const maxSourceBytes = 8 << 20
const maxFileBytes = 256 << 10
const maxEntries = 30000

func Validate(repos []Repository) error {
	names := make(map[string]bool, len(repos))
	if len(repos) > 16 {
		return errors.New("too many code repositories (maximum 16)")
	}
	for _, repo := range repos {
		if !validSlug(repo.Name) || !validSlug(repo.Project) || names[repo.Name] || !filepath.IsAbs(repo.Path) || filepath.Clean(repo.Path) != repo.Path || strings.ContainsRune(repo.Path, 0) {
			return errors.New("invalid code repository configuration")
		}
		names[repo.Name] = true
	}
	return nil
}

func validSlug(s string) bool {
	if s == "" || len(s) > 64 || s[0] == '-' {
		return false
	}
	for _, c := range s {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
			return false
		}
	}
	return true
}

// Validate the whole configured root path, not just the final component. We
// refuse symlinks instead of following an operator-provided indirect target.
func checkRoot(path string) error {
	part := string(filepath.Separator)
	for _, name := range strings.Split(strings.TrimPrefix(path, part), part) {
		if name == "" {
			continue
		}
		part = filepath.Join(part, name)
		info, err := os.Lstat(part)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("configured code repository path is not a real directory")
		}
	}
	return nil
}

// Build scans only .go files at process startup. No background watcher, network
// call, or on-disk index is involved. Restart to obtain a fresh snapshot.
func Build(ctx context.Context, repos []Repository) (*Index, error) {
	if err := Validate(repos); err != nil {
		return nil, err
	}
	index := &Index{Symbols: []Symbol{}, IndexedAt: time.Now().UTC()}
	for _, repo := range repos {
		if err := checkRoot(repo.Path); err != nil {
			return nil, err
		}
		// Root index is metadata-only and held in memory. The configured directory
		// is never used as an output destination.
		root, err := os.OpenRoot(repo.Path)
		if err != nil {
			return nil, errors.New("cannot open configured code repository")
		}
		files, entries, bytesRead := 0, 0, int64(0)
		err = filepath.WalkDir(repo.Path, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			entries++
			if entries > maxEntries {
				return errors.New("code index directory budget exceeded; narrow repository scope")
			}
			if entry.IsDir() {
				if path != repo.Path && (strings.HasPrefix(entry.Name(), ".") || entry.Name() == "vendor" || entry.Name() == "node_modules" || entry.Name() == "testdata") {
					return filepath.SkipDir
				}
				return nil
			}
			if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasPrefix(entry.Name(), ".") {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if info.Size() > maxFileBytes || files >= maxFiles || bytesRead+info.Size() > maxSourceBytes {
				return errors.New("code index budget exceeded; narrow repository scope")
			}
			rel, err := filepath.Rel(repo.Path, path)
			if err != nil {
				return err
			}
			// Reject interior symlinks (including a directory swapped after WalkDir)
			// before opening beneath the pinned repository root.
			parent := ""
			for _, component := range strings.Split(filepath.Dir(rel), string(filepath.Separator)) {
				if component == "." {
					continue
				}
				parent = filepath.Join(parent, component)
				checked, err := root.Lstat(parent)
				if err != nil || !checked.IsDir() || checked.Mode()&os.ModeSymlink != 0 {
					return errors.New("code directory changed during indexing")
				}
			}
			f, err := root.Open(rel)
			if err != nil {
				return err
			}
			st, err := f.Stat()
			if err != nil || !st.Mode().IsRegular() || st.Size() > maxFileBytes {
				f.Close()
				return errors.New("code file changed or is not regular")
			}
			data, err := io.ReadAll(io.LimitReader(f, maxFileBytes+1))
			f.Close()
			if err != nil {
				return err
			}
			if int64(len(data)) != st.Size() {
				return errors.New("code file changed during indexing")
			}
			files++
			bytesRead += st.Size()
			set := token.NewFileSet()
			file, err := parser.ParseFile(set, rel, data, 0)
			if err != nil {
				return nil // Invalid Go files do not become misleading symbols.
			}
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok {
					continue
				}
				name, kind := fn.Name.Name, "function"
				if fn.Recv != nil {
					kind = "method"
				}
				calls := make(map[string]bool)
				ast.Inspect(fn.Body, func(node ast.Node) bool {
					call, ok := node.(*ast.CallExpr)
					if !ok {
						return true
					}
					switch expr := call.Fun.(type) {
					case *ast.Ident:
						calls[expr.Name] = true
					case *ast.SelectorExpr:
						calls[expr.Sel.Name] = true
					}
					return true
				})
				listed := make([]string, 0, len(calls))
				for call := range calls {
					listed = append(listed, call)
				}
				sort.Strings(listed)
				if len(listed) > 40 {
					listed = listed[:40]
				}
				start, end := set.Position(fn.Pos()).Line, set.Position(fn.End()).Line
				index.Symbols = append(index.Symbols, Symbol{Repository: repo.Name, Project: repo.Project, Path: filepath.ToSlash(rel), Name: name, Kind: kind, Line: start, EndLine: end, Calls: listed, Reference: fmt.Sprintf("%s:%s#L%d-L%d", repo.Name, filepath.ToSlash(rel), start, end)})
				if len(index.Symbols) > 20000 {
					return errors.New("code symbol budget exceeded")
				}
			}
			return nil
		})
		root.Close()
		if err != nil {
			return nil, fmt.Errorf("code repository %s: %w", repo.Name, err)
		}
	}
	sort.Slice(index.Symbols, func(i, j int) bool {
		a, b := index.Symbols[i], index.Symbols[j]
		return a.Repository+"/"+a.Path+":"+a.Name < b.Repository+"/"+b.Path+":"+b.Name
	})
	return index, nil
}
