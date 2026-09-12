// Package caseconv converts identifiers and titles between naming
// conventions: snake_case, kebab-case, CamelCase, Title Case and friends.
//
// Every converter is generic over [Text], so the same function works on a
// string or a []byte (and on named types of either) without an extra copy:
//
//	caseconv.ToSnake("AnyKind of_string")            // "any_kind_of_string"
//	caseconv.ToTitle([]byte("quarterly-report.v2"))  // []byte("Quarterly Report V2")
//
// Conversion is a two-step process. [Words] first splits the input into
// words at separators (any rune that is not a letter, digit or apostrophe),
// at lower→upper transitions, at letter↔digit transitions and at the end of
// an acronym ("JSONData" → "JSON", "Data"). The converters then rewrite the
// case of each word and join them with the delimiter of the target
// convention. Letters outside ASCII are handled through the unicode tables,
// so "über straße" and "ÜberStraße" both split into the same two words.
//
// Conversion table for the input "AnyKind of_string":
//
//	| Function                        | Result             |
//	|---------------------------------|--------------------|
//	| ToSnake(s)                      | any_kind_of_string |
//	| ToScreamingSnake(s)             | ANY_KIND_OF_STRING |
//	| ToKebab(s)                      | any-kind-of-string |
//	| ToScreamingKebab(s)             | ANY-KIND-OF-STRING |
//	| ToDelimited(s, '.')             | any.kind.of.string |
//	| ToScreamingDelimited(s, '.')    | ANY.KIND.OF.STRING |
//	| ToCamel(s)                      | AnyKindOfString    |
//	| ToLowerCamel(s)                 | anyKindOfString    |
//	| ToTitle(s)                      | Any Kind Of String |
//	| ToSentence(s)                   | Any kind of string |
//
// [Case] names each convention as a value, for callers that pick the target
// convention at run time (configuration, flags, generated code).
package caseconv
