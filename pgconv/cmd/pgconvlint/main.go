package main

import (
	"github.com/omniaura/go-kit/pgconv/pgconvlint"
	"golang.org/x/tools/go/analysis/singlechecker"
)

func main() {
	singlechecker.Main(pgconvlint.Analyzer)
}
