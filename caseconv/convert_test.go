package caseconv_test

import (
	"fmt"
	"slices"
	"testing"
	"unicode/utf8"

	"github.com/omniaura/go-kit/caseconv"
)

type pair struct{ in, want string }

// check runs fn over the pairs on both the string and []byte instantiations
// and verifies they agree.
func check(t *testing.T, name string, fn func(string) string, fnBytes func([]byte) []byte, cases []pair) {
	t.Helper()
	for _, c := range cases {
		if got := fn(c.in); got != c.want {
			t.Errorf("%s(%q) = %q, want %q", name, c.in, got, c.want)
		}
		if got := string(fnBytes([]byte(c.in))); got != c.want {
			t.Errorf("%s([]byte(%q)) = %q, want %q", name, c.in, got, c.want)
		}
	}
}

func TestWords(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"   ", nil},
		{"test", []string{"test"}},
		{"testCase", []string{"test", "Case"}},
		{"TestCase", []string{"Test", "Case"}},
		{" Test  Case ", []string{"Test", "Case"}},
		{"test_case-kebab.dot", []string{"test", "case", "kebab", "dot"}},
		{"JSONData", []string{"JSON", "Data"}},
		{"userID", []string{"user", "ID"}},
		{"AAAbbb", []string{"AA", "Abbb"}},
		{"numbers2and55with000", []string{"numbers", "2", "and", "55", "with", "000"}},
		{"AB1AB2AB3", []string{"AB", "1", "AB", "2", "AB", "3"}},
		{"don't stop", []string{"don't", "stop"}},
		{"über straße", []string{"über", "straße"}},
		{"ÜberStraße", []string{"Über", "Straße"}},
		{"HTMLÉlément", []string{"HTML", "Élément"}},
		{"日本語 text", []string{"日本語", "text"}},
		{"a@b.c", []string{"a", "b", "c"}},
		{"\xff\xfe broken", []string{"broken"}},
	}
	for _, c := range cases {
		got := caseconv.Words(c.in)
		if !slices.Equal(got, c.want) {
			t.Errorf("Words(%q) = %q, want %q", c.in, got, c.want)
		}
		gotBytes := caseconv.Words([]byte(c.in))
		if len(gotBytes) != len(c.want) {
			t.Errorf("Words([]byte(%q)) = %q, want %q", c.in, gotBytes, c.want)
			continue
		}
		for i := range gotBytes {
			if string(gotBytes[i]) != c.want[i] {
				t.Errorf("Words([]byte(%q))[%d] = %q, want %q", c.in, i, gotBytes[i], c.want[i])
			}
		}
	}
}

func TestSplitStopsEarly(t *testing.T) {
	var got []string
	for w := range caseconv.Split("one two three") {
		got = append(got, w)
		if len(got) == 2 {
			break
		}
	}
	if want := []string{"one", "two"}; !slices.Equal(got, want) {
		t.Errorf("Split stopped early = %q, want %q", got, want)
	}
}

func TestToSnake(t *testing.T) {
	check(t, "ToSnake", caseconv.ToSnake[string], caseconv.ToSnake[[]byte], []pair{
		{"testCase", "test_case"},
		{"TestCase", "test_case"},
		{"Test Case", "test_case"},
		{" Test Case", "test_case"},
		{"Test Case ", "test_case"},
		{" Test Case ", "test_case"},
		{"test", "test"},
		{"test_case", "test_case"},
		{"Test", "test"},
		{"", ""},
		{"ManyManyWords", "many_many_words"},
		{"manyManyWords", "many_many_words"},
		{"AnyKind of_string", "any_kind_of_string"},
		{"numbers2and55with000", "numbers_2_and_55_with_000"},
		{"JSONData", "json_data"},
		{"userID", "user_id"},
		{"AAAbbb", "aa_abbb"},
		{"1A2", "1_a_2"},
		{"A1B", "a_1_b"},
		{"A1A2A3", "a_1_a_2_a_3"},
		{"A1 A2 A3", "a_1_a_2_a_3"},
		{"AB1AB2AB3", "ab_1_ab_2_ab_3"},
		{"AB1 AB2 AB3", "ab_1_ab_2_ab_3"},
		{"some string", "some_string"},
		{" some string", "some_string"},
		{"test-case", "test_case"},
		{"__leading__and__trailing__", "leading_and_trailing"},
		{"ÜberStraße", "über_straße"},
		{"HTMLÉlément", "html_élément"},
	})
}

func TestToScreamingSnake(t *testing.T) {
	check(t, "ToScreamingSnake", caseconv.ToScreamingSnake[string], caseconv.ToScreamingSnake[[]byte], []pair{
		{"testCase", "TEST_CASE"},
		{"über straße", "ÜBER_STRASSE"},
	})
}

func TestToKebab(t *testing.T) {
	check(t, "ToKebab", caseconv.ToKebab[string], caseconv.ToKebab[[]byte], []pair{
		{"testCase", "test-case"},
		{"Hello, World!", "hello-world"},
	})
}

func TestToScreamingKebab(t *testing.T) {
	check(t, "ToScreamingKebab", caseconv.ToScreamingKebab[string], caseconv.ToScreamingKebab[[]byte], []pair{
		{"testCase", "TEST-CASE"},
	})
}

func TestToDelimited(t *testing.T) {
	fn := func(s string) string { return caseconv.ToDelimited(s, "@") }
	fnBytes := func(s []byte) []byte { return caseconv.ToDelimited(s, "@") }
	check(t, "ToDelimited", fn, fnBytes, []pair{
		{"testCase", "test@case"},
		{"TestCase", "test@case"},
		{"Test Case", "test@case"},
		{"test", "test"},
		{"test_case", "test@case"},
		{"", ""},
		{"ManyManyWords", "many@many@words"},
		{"AnyKind of_string", "any@kind@of@string"},
		{"numbers2and55with000", "numbers@2@and@55@with@000"},
		{"JSONData", "json@data"},
		{"userID", "user@id"},
		{"AAAbbb", "aa@abbb"},
		{"test-case", "test@case"},
	})
	if got := caseconv.ToDelimited("a b", "::"); got != "a::b" {
		t.Errorf("multi-byte delimiter: got %q", got)
	}
}

func TestToScreamingDelimited(t *testing.T) {
	fn := func(s string) string { return caseconv.ToScreamingDelimited(s, ".") }
	fnBytes := func(s []byte) []byte { return caseconv.ToScreamingDelimited(s, ".") }
	check(t, "ToScreamingDelimited", fn, fnBytes, []pair{
		{"testCase", "TEST.CASE"},
		{"AnyKind of_string", "ANY.KIND.OF.STRING"},
	})
}

func TestToCamel(t *testing.T) {
	check(t, "ToCamel", caseconv.ToCamel[string], caseconv.ToCamel[[]byte], []pair{
		{"test_case", "TestCase"},
		{"test.case", "TestCase"},
		{"test", "Test"},
		{"TestCase", "TestCase"},
		{" test  case ", "TestCase"},
		{"", ""},
		{"many_many_words", "ManyManyWords"},
		{"AnyKind of_string", "AnyKindOfString"},
		{"odd-fix", "OddFix"},
		{"numbers2And55with000", "Numbers2And55With000"},
		{"ID", "Id"},
		{"CONSTANT_CASE", "ConstantCase"},
		{"über straße", "ÜberStraße"},
	})
}

func TestToLowerCamel(t *testing.T) {
	check(t, "ToLowerCamel", caseconv.ToLowerCamel[string], caseconv.ToLowerCamel[[]byte], []pair{
		{"foo-bar", "fooBar"},
		{"TestCase", "testCase"},
		{"", ""},
		{"AnyKind of_string", "anyKindOfString"},
		{"AnyKind.of-string", "anyKindOfString"},
		{"ID", "id"},
		{"some string", "someString"},
		{" some string", "someString"},
		{"CONSTANT_CASE", "constantCase"},
	})
}

func TestToTitle(t *testing.T) {
	check(t, "ToTitle", caseconv.ToTitle[string], caseconv.ToTitle[[]byte], []pair{
		{"", ""},
		{"integration-channel-partner-compensation-benchmarks", "Integration Channel Partner Compensation Benchmarks"},
		{"quarterly_report_v2", "Quarterly Report V 2"},
		{"AnyKind of_string", "Any Kind Of String"},
		{"API-reference", "API Reference"},
		{"already Title Cased", "Already Title Cased"},
		{"don't-panic", "Don't Panic"},
		{"über_straße", "Über Straße"},
		{"README", "README"},
		{"2024-plan", "2024 Plan"},
	})
}

func TestToSentence(t *testing.T) {
	check(t, "ToSentence", caseconv.ToSentence[string], caseconv.ToSentence[[]byte], []pair{
		{"", ""},
		{"hello_world", "Hello world"},
		{"AnyKind of_string", "Any Kind of string"},
		{"API-reference", "API reference"},
		{"über_straße", "Über straße"},
	})
}

func TestNamedTypes(t *testing.T) {
	type slug string
	type raw []byte
	if got := caseconv.ToTitle(slug("a-b")); got != slug("A B") {
		t.Errorf("named string type: got %q", got)
	}
	if got := caseconv.ToKebab(raw("A B")); string(got) != "a-b" {
		t.Errorf("named byte type: got %q", got)
	}
}

func TestDoesNotAliasInput(t *testing.T) {
	in := []byte("hello world")
	out := caseconv.ToTitle(in)
	out[0] = 'X'
	if string(in) != "hello world" {
		t.Errorf("output aliases input: %q", in)
	}
}

func TestCase(t *testing.T) {
	const in = "AnyKind of_string"
	want := map[caseconv.Case]string{
		caseconv.Unknown:        in,
		caseconv.Snake:          "any_kind_of_string",
		caseconv.ScreamingSnake: "ANY_KIND_OF_STRING",
		caseconv.Kebab:          "any-kind-of-string",
		caseconv.ScreamingKebab: "ANY-KIND-OF-STRING",
		caseconv.Camel:          "AnyKindOfString",
		caseconv.LowerCamel:     "anyKindOfString",
		caseconv.Title:          "Any Kind Of String",
		caseconv.Sentence:       "Any Kind of string",
	}
	for c, w := range want {
		if got := c.Convert(in); got != w {
			t.Errorf("%v.Convert(%q) = %q, want %q", c, in, got, w)
		}
		if got := string(c.ConvertBytes([]byte(in))); got != w {
			t.Errorf("%v.ConvertBytes(%q) = %q, want %q", c, in, got, w)
		}
	}
	if got := caseconv.Case(200).Convert(in); got != in {
		t.Errorf("out-of-range Case changed input: %q", got)
	}
	if got := caseconv.Case(200).String(); got != "Case(200)" {
		t.Errorf("out-of-range String = %q", got)
	}
	if _, err := caseconv.Case(200).MarshalText(); err == nil {
		t.Error("out-of-range MarshalText succeeded")
	}
	for _, c := range caseconv.Cases() {
		if _, ok := want[c]; !ok {
			t.Errorf("Cases() returned %v which has no conversion", c)
		}
		text, err := c.MarshalText()
		if err != nil {
			t.Fatalf("%v.MarshalText: %v", c, err)
		}
		var back caseconv.Case
		if err := back.UnmarshalText(text); err != nil {
			t.Fatalf("UnmarshalText(%q): %v", text, err)
		}
		if back != c {
			t.Errorf("round trip %v -> %q -> %v", c, text, back)
		}
	}
}

func TestParseCase(t *testing.T) {
	for name, want := range map[string]caseconv.Case{
		"snake":           caseconv.Snake,
		"SNAKE":           caseconv.Snake,
		"screaming_snake": caseconv.ScreamingSnake,
		"screaming-snake": caseconv.ScreamingSnake,
		"Screaming Snake": caseconv.ScreamingSnake,
		"ScreamingSnake":  caseconv.ScreamingSnake,
		" lower_camel ":   caseconv.LowerCamel,
		"title":           caseconv.Title,
	} {
		got, err := caseconv.ParseCase(name)
		if err != nil {
			t.Errorf("ParseCase(%q): %v", name, err)
			continue
		}
		if got != want {
			t.Errorf("ParseCase(%q) = %v, want %v", name, got, want)
		}
	}
	for _, name := range []string{"", "unknown", "pascal"} {
		if got, err := caseconv.ParseCase(name); err == nil {
			t.Errorf("ParseCase(%q) = %v, want error", name, got)
		}
	}
}

func FuzzConvert(f *testing.F) {
	for _, seed := range []string{"", "testCase", "JSONData v2.1", "über straße", "AB1AB2AB3", "\xff\xfe", "日本語 text"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, in string) {
		for _, c := range caseconv.Cases() {
			got := c.Convert(in)
			if utf8.ValidString(in) && !utf8.ValidString(got) {
				t.Errorf("%v.Convert(%q) produced invalid UTF-8 %q", c, in, got)
			}
			if gotBytes := string(c.ConvertBytes([]byte(in))); gotBytes != got {
				t.Errorf("%v: string and []byte differ: %q vs %q", c, got, gotBytes)
			}
			// Converting twice must be a fixed point for every delimiter
			// convention; the words survive the first pass unchanged.
			if c != caseconv.Camel && c != caseconv.LowerCamel {
				if again := c.Convert(got); again != got {
					t.Errorf("%v not idempotent: %q -> %q -> %q", c, in, got, again)
				}
			}
		}
	})
}

func ExampleToTitle() {
	fmt.Println(caseconv.ToTitle("integration-channel-partner-compensation-benchmarks"))
	fmt.Println(caseconv.ToTitle("API-reference.v2"))
	// Output:
	// Integration Channel Partner Compensation Benchmarks
	// API Reference V 2
}

func ExampleWords() {
	fmt.Printf("%q\n", caseconv.Words("JSONData v2.1"))
	// Output:
	// ["JSON" "Data" "v" "2" "1"]
}

func ExampleCase_Convert() {
	c, _ := caseconv.ParseCase("screaming-kebab")
	fmt.Println(c, c.Convert("AnyKind of_string"))
	// Output:
	// screaming_kebab ANY-KIND-OF-STRING
}

var sink string

func benchmarkConvert(b *testing.B, fn func(string) string) {
	inputs := []string{"testCase", "AnyKind of_string", "numbers2and55with000", "JSONData", "integration-channel-partner-compensation-benchmarks"}
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		sink = fn(inputs[i%len(inputs)])
	}
}

func BenchmarkToSnake(b *testing.B) { benchmarkConvert(b, caseconv.ToSnake[string]) }
func BenchmarkToCamel(b *testing.B) { benchmarkConvert(b, caseconv.ToCamel[string]) }
func BenchmarkToTitle(b *testing.B) { benchmarkConvert(b, caseconv.ToTitle[string]) }
func BenchmarkWords(b *testing.B) {
	benchmarkConvert(b, func(s string) string { return caseconv.Words(s)[0] })
}
