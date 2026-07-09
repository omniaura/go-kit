package main

import (
	"github.com/omniaura/go-kit/lookuptablelint"
	"golang.org/x/tools/go/analysis/singlechecker"
)

func main() {
	singlechecker.Main(lookuptablelint.Analyzer)
}
