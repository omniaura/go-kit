// Package pgconvlint provides a Go analyzer for finding manual pgtype
// conversions that should use pgencode or pgdecode.
//
// The analyzer reports direct pgtype composite-literal encodes, guarded
// pgtype field decodes, adjacent value/Valid field assignments, and tiny
// package-local pgconv wrapper helpers. Safe mechanical diagnostics include
// SuggestedFix values that editors and compatible analyzer drivers can apply.
package pgconvlint
