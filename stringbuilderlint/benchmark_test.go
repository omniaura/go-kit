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
	benchF       = "foxtrot"
	benchG       = "golf"
	benchH       = "hotel"
	benchI       = "india"
	benchJ       = "juliet"
	benchK       = "kilo"
	benchL       = "lima"
	benchM       = "mike"
	benchN       = "november"
	benchO       = "oscar"
	benchP       = "papa"
	benchQ       = "quebec"
	benchR       = "romeo"
	benchS       = "sierra"
	benchT       = "tango"
	benchU       = "uniform"
	benchV       = "victor"
	benchW       = "whiskey"
	benchX       = "xray"
	benchY       = "yankee"
	benchZ       = "zulu"
	benchAA      = "alpha-alpha"
	benchAB      = "alpha-bravo"
	benchAC      = "alpha-charlie"
	benchAD      = "alpha-delta"
	benchAE      = "alpha-echo"
	benchAF      = "alpha-foxtrot"
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

func BenchmarkConcat4(b *testing.B) {
	for b.Loop() {
		benchmarkStringSink = benchA + benchB + benchC + benchD
	}
}

func BenchmarkBuilderConcat4(b *testing.B) {
	for b.Loop() {
		part0 := benchA
		part1 := benchB
		part2 := benchC
		part3 := benchD
		var sb strings.Builder
		sb.Grow(len(part0) + len(part1) + len(part2) + len(part3))
		sb.WriteString(part0)
		sb.WriteString(part1)
		sb.WriteString(part2)
		sb.WriteString(part3)
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

func BenchmarkConcat8(b *testing.B) {
	for b.Loop() {
		benchmarkStringSink = benchA + benchB + benchC + benchD + benchE + benchF + benchG + benchH
	}
}

func BenchmarkBuilderConcat8(b *testing.B) {
	for b.Loop() {
		part0 := benchA
		part1 := benchB
		part2 := benchC
		part3 := benchD
		part4 := benchE
		part5 := benchF
		part6 := benchG
		part7 := benchH
		var sb strings.Builder
		sb.Grow(len(part0) + len(part1) + len(part2) + len(part3) + len(part4) + len(part5) + len(part6) + len(part7))
		sb.WriteString(part0)
		sb.WriteString(part1)
		sb.WriteString(part2)
		sb.WriteString(part3)
		sb.WriteString(part4)
		sb.WriteString(part5)
		sb.WriteString(part6)
		sb.WriteString(part7)
		benchmarkStringSink = sb.String()
	}
}

func BenchmarkConcat16(b *testing.B) {
	for b.Loop() {
		benchmarkStringSink = benchA + benchB + benchC + benchD + benchE + benchF + benchG + benchH + benchI + benchJ + benchK + benchL + benchM + benchN + benchO + benchP
	}
}

func BenchmarkBuilderConcat16(b *testing.B) {
	for b.Loop() {
		part0 := benchA
		part1 := benchB
		part2 := benchC
		part3 := benchD
		part4 := benchE
		part5 := benchF
		part6 := benchG
		part7 := benchH
		part8 := benchI
		part9 := benchJ
		part10 := benchK
		part11 := benchL
		part12 := benchM
		part13 := benchN
		part14 := benchO
		part15 := benchP
		var sb strings.Builder
		sb.Grow(len(part0) + len(part1) + len(part2) + len(part3) + len(part4) + len(part5) + len(part6) + len(part7) + len(part8) + len(part9) + len(part10) + len(part11) + len(part12) + len(part13) + len(part14) + len(part15))
		sb.WriteString(part0)
		sb.WriteString(part1)
		sb.WriteString(part2)
		sb.WriteString(part3)
		sb.WriteString(part4)
		sb.WriteString(part5)
		sb.WriteString(part6)
		sb.WriteString(part7)
		sb.WriteString(part8)
		sb.WriteString(part9)
		sb.WriteString(part10)
		sb.WriteString(part11)
		sb.WriteString(part12)
		sb.WriteString(part13)
		sb.WriteString(part14)
		sb.WriteString(part15)
		benchmarkStringSink = sb.String()
	}
}

func BenchmarkConcat32(b *testing.B) {
	for b.Loop() {
		benchmarkStringSink = benchA + benchB + benchC + benchD + benchE + benchF + benchG + benchH + benchI + benchJ + benchK + benchL + benchM + benchN + benchO + benchP + benchQ + benchR + benchS + benchT + benchU + benchV + benchW + benchX + benchY + benchZ + benchAA + benchAB + benchAC + benchAD + benchAE + benchAF
	}
}

func BenchmarkBuilderConcat32(b *testing.B) {
	for b.Loop() {
		part0 := benchA
		part1 := benchB
		part2 := benchC
		part3 := benchD
		part4 := benchE
		part5 := benchF
		part6 := benchG
		part7 := benchH
		part8 := benchI
		part9 := benchJ
		part10 := benchK
		part11 := benchL
		part12 := benchM
		part13 := benchN
		part14 := benchO
		part15 := benchP
		part16 := benchQ
		part17 := benchR
		part18 := benchS
		part19 := benchT
		part20 := benchU
		part21 := benchV
		part22 := benchW
		part23 := benchX
		part24 := benchY
		part25 := benchZ
		part26 := benchAA
		part27 := benchAB
		part28 := benchAC
		part29 := benchAD
		part30 := benchAE
		part31 := benchAF
		var sb strings.Builder
		sb.Grow(len(part0) + len(part1) + len(part2) + len(part3) + len(part4) + len(part5) + len(part6) + len(part7) + len(part8) + len(part9) + len(part10) + len(part11) + len(part12) + len(part13) + len(part14) + len(part15) + len(part16) + len(part17) + len(part18) + len(part19) + len(part20) + len(part21) + len(part22) + len(part23) + len(part24) + len(part25) + len(part26) + len(part27) + len(part28) + len(part29) + len(part30) + len(part31))
		sb.WriteString(part0)
		sb.WriteString(part1)
		sb.WriteString(part2)
		sb.WriteString(part3)
		sb.WriteString(part4)
		sb.WriteString(part5)
		sb.WriteString(part6)
		sb.WriteString(part7)
		sb.WriteString(part8)
		sb.WriteString(part9)
		sb.WriteString(part10)
		sb.WriteString(part11)
		sb.WriteString(part12)
		sb.WriteString(part13)
		sb.WriteString(part14)
		sb.WriteString(part15)
		sb.WriteString(part16)
		sb.WriteString(part17)
		sb.WriteString(part18)
		sb.WriteString(part19)
		sb.WriteString(part20)
		sb.WriteString(part21)
		sb.WriteString(part22)
		sb.WriteString(part23)
		sb.WriteString(part24)
		sb.WriteString(part25)
		sb.WriteString(part26)
		sb.WriteString(part27)
		sb.WriteString(part28)
		sb.WriteString(part29)
		sb.WriteString(part30)
		sb.WriteString(part31)
		benchmarkStringSink = sb.String()
	}
}

func BenchmarkSprintfOnePrimitive(b *testing.B) {
	for b.Loop() {
		benchmarkStringSink = fmt.Sprintf("id=%d", benchCount)
	}
}

func BenchmarkBuilderSprintfOnePrimitive(b *testing.B) {
	for b.Loop() {
		part1 := strconv.FormatInt(int64(benchCount), 10)
		var sb strings.Builder
		sb.Grow(3 + len(part1))
		sb.WriteString("id=")
		sb.WriteString(part1)
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
