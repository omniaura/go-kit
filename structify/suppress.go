package structify

import (
	"go/ast"
	"go/token"
	"os"
	"strings"
)

// Suppression comments honored, in either spelling:
//
//	//lint:ignore structify <reason>        (line above the declaration)
//	//lint:file-ignore structify <reason>   (whole file)
//	// structify:ignore <reason>            (line above the declaration)
//
// The legacy analyzer name "funcparamlint" is accepted anywhere "structify"
// is, so existing suppressions keep working after a swap.
const analyzerName = "structify"

const legacyName = "funcparamlint"

type suppressions struct {
	lines map[int]bool
	file  bool
}

func parseSuppressions(fset *token.FileSet, f *ast.File) suppressions {
	s := suppressions{lines: map[int]bool{}}
	for _, group := range f.Comments {
		for _, c := range group.List {
			text := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(c.Text, "//"), "/*"))
			fields := strings.Fields(text)
			if len(fields) < 2 {
				continue
			}
			switch fields[0] {
			case "lint:ignore":
				if nameMatches(fields[1]) {
					s.lines[fset.Position(c.End()).Line+1] = true
				}
			case "lint:file-ignore":
				if nameMatches(fields[1]) {
					s.file = true
				}
			case analyzerName + ":ignore":
				s.lines[fset.Position(c.End()).Line+1] = true
			}
		}
	}
	return s
}

func nameMatches(list string) bool {
	for _, name := range strings.Split(list, ",") {
		name = strings.TrimSpace(name)
		if name == analyzerName || name == legacyName {
			return true
		}
	}
	return false
}

func (s suppressions) ignored(fset *token.FileSet, pos token.Pos) bool {
	return s.file || s.lines[fset.Position(pos).Line]
}

func readFile(name string) ([]byte, error) { return os.ReadFile(name) }
