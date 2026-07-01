// Package lintignore parses standard Go lint suppression comments.
package lintignore

import (
	"go/ast"
	"go/token"
	"strings"
)

// Suppressions stores //lint:ignore and //lint:file-ignore directives for a file.
type Suppressions struct {
	fset *token.FileSet
	line map[int]map[string]bool
	file map[string]bool
}

// New parses lint suppression directives from file comments.
func New(fset *token.FileSet, file *ast.File) Suppressions {
	s := Suppressions{
		fset: fset,
		line: make(map[int]map[string]bool),
		file: make(map[string]bool),
	}

	for _, group := range file.Comments {
		for _, comment := range group.List {
			kind, analyzers, ok := parseDirective(comment.Text)
			if !ok {
				continue
			}

			switch kind {
			case "lint:ignore":
				endLine := fset.PositionFor(comment.End(), false).Line
				s.addLine(endLine+1, analyzers)
			case "lint:file-ignore":
				for _, analyzer := range analyzers {
					s.file[analyzer] = true
				}
			}
		}
	}

	return s
}

// Ignored reports whether analyzer diagnostics at pos should be suppressed.
func (s Suppressions) Ignored(pos token.Pos, analyzer string) bool {
	if matches(s.file, analyzer) {
		return true
	}

	line := s.fset.PositionFor(pos, false).Line
	return matches(s.line[line], analyzer)
}

func (s Suppressions) addLine(line int, analyzers []string) {
	if line <= 0 {
		return
	}
	if s.line[line] == nil {
		s.line[line] = make(map[string]bool)
	}
	for _, analyzer := range analyzers {
		s.line[line][analyzer] = true
	}
}

func parseDirective(raw string) (string, []string, bool) {
	text := strings.TrimSpace(commentText(raw))
	fields := strings.Fields(text)
	if len(fields) < 3 {
		return "", nil, false
	}

	kind := fields[0]
	if kind != "lint:ignore" && kind != "lint:file-ignore" {
		return "", nil, false
	}

	var analyzers []string
	for _, analyzer := range strings.Split(fields[1], ",") {
		analyzer = strings.TrimSpace(analyzer)
		if analyzer != "" {
			analyzers = append(analyzers, analyzer)
		}
	}
	return kind, analyzers, len(analyzers) > 0
}

func commentText(raw string) string {
	switch {
	case strings.HasPrefix(raw, "//"):
		return strings.TrimSpace(strings.TrimPrefix(raw, "//"))
	case strings.HasPrefix(raw, "/*") && strings.HasSuffix(raw, "*/"):
		raw = strings.TrimPrefix(raw, "/*")
		raw = strings.TrimSuffix(raw, "*/")
		return strings.TrimSpace(raw)
	default:
		return strings.TrimSpace(raw)
	}
}

func matches(analyzers map[string]bool, analyzer string) bool {
	return analyzers[analyzer] || analyzers["all"] || analyzers["*"]
}
