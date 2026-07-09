package stringbuilderlint

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
)

var benchmarkStringSink string

var (
	benchA       = "alpha"
	benchB       = "bravo"
	benchC       = "charlie"
	benchD       = "delta"
	benchE       = "echo"
	benchCount   = 12345
	benchEnabled = true
)

func BenchmarkConcat3(b *testing.B) {
	for b.Loop() {
		benchmarkStringSink = benchA + benchB + benchC
	}
}

func BenchmarkBuilderConcat3(b *testing.B) {
	for b.Loop() {
		part0 := benchA
		part1 := benchB
		part2 := benchC
		var sb strings.Builder
		sb.Grow(len(part0) + len(part1) + len(part2))
		sb.WriteString(part0)
		sb.WriteString(part1)
		sb.WriteString(part2)
		benchmarkStringSink = sb.String()
	}
}

func BenchmarkConcat5(b *testing.B) {
	for b.Loop() {
		benchmarkStringSink = benchA + benchB + benchC + benchD + benchE
	}
}

func BenchmarkBuilderConcat5(b *testing.B) {
	for b.Loop() {
		part0 := benchA
		part1 := benchB
		part2 := benchC
		part3 := benchD
		part4 := benchE
		var sb strings.Builder
		sb.Grow(len(part0) + len(part1) + len(part2) + len(part3) + len(part4))
		sb.WriteString(part0)
		sb.WriteString(part1)
		sb.WriteString(part2)
		sb.WriteString(part3)
		sb.WriteString(part4)
		benchmarkStringSink = sb.String()
	}
}

func BenchmarkSprintfPrimitive(b *testing.B) {
	for b.Loop() {
		benchmarkStringSink = fmt.Sprintf("%s/%d/%t", benchA, benchCount, benchEnabled)
	}
}

func BenchmarkBuilderSprintfPrimitive(b *testing.B) {
	for b.Loop() {
		part0 := benchA
		part2 := strconv.FormatInt(int64(benchCount), 10)
		part4 := strconv.FormatBool(benchEnabled)
		var sb strings.Builder
		sb.Grow(len(part0) + 1 + len(part2) + 1 + len(part4))
		sb.WriteString(part0)
		sb.WriteString("/")
		sb.WriteString(part2)
		sb.WriteString("/")
		sb.WriteString(part4)
		benchmarkStringSink = sb.String()
	}
}
