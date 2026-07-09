package lookuptablelint

import "testing"

var benchmarkBoolSink bool

var lookupInputs = [...]string{
	"a", "b", "c", "d", "e", "f", "g", "h",
	"i", "j", "k", "l", "m", "n", "o", "p",
}

var lookupMap2 = map[string]struct{}{
	"a": {},
	"b": {},
}

var lookupMap4 = map[string]struct{}{
	"a": {},
	"b": {},
	"c": {},
	"d": {},
}

var lookupMap8 = map[string]struct{}{
	"a": {},
	"b": {},
	"c": {},
	"d": {},
	"e": {},
	"f": {},
	"g": {},
	"h": {},
}

var lookupMap16 = map[string]struct{}{
	"a": {},
	"b": {},
	"c": {},
	"d": {},
	"e": {},
	"f": {},
	"g": {},
	"h": {},
	"i": {},
	"j": {},
	"k": {},
	"l": {},
	"m": {},
	"n": {},
	"o": {},
	"p": {},
}

func lookupSwitch2(value string) bool {
	switch value {
	case "a", "b":
		return true
	default:
		return false
	}
}

func lookupSwitch4(value string) bool {
	switch value {
	case "a", "b", "c", "d":
		return true
	default:
		return false
	}
}

func lookupSwitch8(value string) bool {
	switch value {
	case "a", "b", "c", "d", "e", "f", "g", "h":
		return true
	default:
		return false
	}
}

func lookupSwitch16(value string) bool {
	switch value {
	case "a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "m", "n", "o", "p":
		return true
	default:
		return false
	}
}

func BenchmarkMapLookup2(b *testing.B) {
	for i := 0; b.Loop(); i++ {
		_, benchmarkBoolSink = lookupMap2[lookupInputs[i&15]]
	}
}

func BenchmarkSwitchLookup2(b *testing.B) {
	for i := 0; b.Loop(); i++ {
		benchmarkBoolSink = lookupSwitch2(lookupInputs[i&15])
	}
}

func BenchmarkMapLookup4(b *testing.B) {
	for i := 0; b.Loop(); i++ {
		_, benchmarkBoolSink = lookupMap4[lookupInputs[i&15]]
	}
}

func BenchmarkSwitchLookup4(b *testing.B) {
	for i := 0; b.Loop(); i++ {
		benchmarkBoolSink = lookupSwitch4(lookupInputs[i&15])
	}
}

func BenchmarkMapLookup8(b *testing.B) {
	for i := 0; b.Loop(); i++ {
		_, benchmarkBoolSink = lookupMap8[lookupInputs[i&15]]
	}
}

func BenchmarkSwitchLookup8(b *testing.B) {
	for i := 0; b.Loop(); i++ {
		benchmarkBoolSink = lookupSwitch8(lookupInputs[i&15])
	}
}

func BenchmarkMapLookup16(b *testing.B) {
	for i := 0; b.Loop(); i++ {
		_, benchmarkBoolSink = lookupMap16[lookupInputs[i&15]]
	}
}

func BenchmarkSwitchLookup16(b *testing.B) {
	for i := 0; b.Loop(); i++ {
		benchmarkBoolSink = lookupSwitch16(lookupInputs[i&15])
	}
}
