# caseconv

Convert identifiers and titles between naming conventions. Every function is
generic over `string` and `[]byte` (and named types of either), Unicode-aware,
and allocates exactly once.

```go
import "github.com/omniaura/go-kit/caseconv"

s := "AnyKind of_string"
```

| Function                        | Result               |
|---------------------------------|----------------------|
| `ToSnake(s)`                    | `any_kind_of_string` |
| `ToScreamingSnake(s)`           | `ANY_KIND_OF_STRING` |
| `ToKebab(s)`                    | `any-kind-of-string` |
| `ToScreamingKebab(s)`           | `ANY-KIND-OF-STRING` |
| `ToDelimited(s, ".")`           | `any.kind.of.string` |
| `ToScreamingDelimited(s, ".")`  | `ANY.KIND.OF.STRING` |
| `ToCamel(s)`                    | `AnyKindOfString`    |
| `ToLowerCamel(s)`               | `anyKindOfString`    |
| `ToTitle(s)`                    | `Any Kind Of String` |
| `ToSentence(s)`                 | `Any kind of string` |

`Words(s)` / `Split(s)` expose the word splitter: `Words("JSONData v2.1")` is
`["JSON" "Data" "v" "2" "1"]`. `Case` names each convention as a value
(`caseconv.ParseCase("screaming-kebab")`, `c.Convert(s)`) and round-trips
through `encoding.TextMarshaler`.

Ported from [peyton-spencer/caseconv](https://github.com/peyton-spencer/caseconv)
(itself derived from [iancoleman/strcase](https://github.com/iancoleman/strcase),
MIT). Differences from the original: one generic implementation instead of
parallel `strcase`/`bytcase` packages, Unicode letters are classified through
the `unicode` tables instead of being passed through opaquely, every
non-letter/digit rune is a separator (not only `_ - . space`), apostrophes stay
inside words, and `ToTitle`/`ToSentence`/`Words` are new. The `ignore`
parameter was dropped: split with `Words` and join yourself when a delimiter
must survive.
