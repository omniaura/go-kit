package pgencode_test

import (
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/omniaura/go-kit/pgconv/pgencode"
)

func TestString(t *testing.T) {
	if got := pgencode.String("hello").Text(); !got.Valid || got.String != "hello" {
		t.Fatalf("String().Text() = %#v", got)
	}
	if got := pgencode.String("").Text(); !got.Valid || got.String != "" {
		t.Fatalf("String(empty).Text() = %#v", got)
	}
	if got := pgencode.String("").EmptyIsNull().Text(); got.Valid {
		t.Fatalf("String(empty).EmptyIsNull().Text() = %#v", got)
	}
	if got := pgencode.StringPtr((*string)(nil)).Text(); got.Valid {
		t.Fatalf("StringPtr(nil).Text() = %#v", got)
	}

	value := "world"
	if got := pgencode.StringPtr(&value).Text(); !got.Valid || got.String != "world" {
		t.Fatalf("StringPtr().Text() = %#v", got)
	}
}

func TestStringTrimSpace(t *testing.T) {
	if got := pgencode.String("  hi  ").TrimSpace().Text(); !got.Valid || got.String != "hi" {
		t.Fatalf("String(pad).TrimSpace().Text() = %#v", got)
	}
	// Composes with EmptyIsNull in either order; all-whitespace -> NULL.
	if got := pgencode.String("   ").TrimSpace().EmptyIsNull().Text(); got.Valid {
		t.Fatalf("String(ws).TrimSpace().EmptyIsNull().Text() = %#v", got)
	}
	if got := pgencode.String("   ").EmptyIsNull().TrimSpace().Text(); got.Valid {
		t.Fatalf("String(ws).EmptyIsNull().TrimSpace().Text() = %#v", got)
	}
	// Trimmed-but-non-empty stays valid and carries the trimmed value.
	if got := pgencode.String("\t x \n").TrimSpace().EmptyIsNull().Text(); !got.Valid || got.String != "x" {
		t.Fatalf("String(pad).TrimSpace().EmptyIsNull().Text() = %#v", got)
	}
	// Without EmptyIsNull, an all-whitespace input trims to a valid empty string.
	if got := pgencode.String("   ").TrimSpace().Text(); !got.Valid || got.String != "" {
		t.Fatalf("String(ws).TrimSpace().Text() = %#v", got)
	}
	// StringPtr(nil) stays NULL regardless of TrimSpace.
	if got := pgencode.StringPtr((*string)(nil)).TrimSpace().EmptyIsNull().Text(); got.Valid {
		t.Fatalf("StringPtr(nil).TrimSpace().EmptyIsNull().Text() = %#v", got)
	}
}

func TestIntegersNonPositiveIsNull(t *testing.T) {
	// Positive values stay valid at every output width.
	if got := pgencode.Int64(5).NonPositiveIsNull().Int8(); !got.Valid || got.Int64 != 5 {
		t.Fatalf("Int64(5).NonPositiveIsNull().Int8() = %#v", got)
	}
	if got := pgencode.Int64(5).NonPositiveIsNull().Int4(); !got.Valid || got.Int32 != 5 {
		t.Fatalf("Int64(5).NonPositiveIsNull().Int4() = %#v", got)
	}
	if got := pgencode.Int64(5).NonPositiveIsNull().Int2(); !got.Valid || got.Int16 != 5 {
		t.Fatalf("Int64(5).NonPositiveIsNull().Int2() = %#v", got)
	}

	// Zero is NULL at every output width.
	if got := pgencode.Int64(0).NonPositiveIsNull().Int8(); got.Valid {
		t.Fatalf("Int64(0).NonPositiveIsNull().Int8() = %#v", got)
	}
	if got := pgencode.Int64(0).NonPositiveIsNull().Int4(); got.Valid {
		t.Fatalf("Int64(0).NonPositiveIsNull().Int4() = %#v", got)
	}
	if got := pgencode.Int64(0).NonPositiveIsNull().Int2(); got.Valid {
		t.Fatalf("Int64(0).NonPositiveIsNull().Int2() = %#v", got)
	}

	// Negative values are NULL too (this is the difference from ZeroIsNull).
	if got := pgencode.Int64(-3).NonPositiveIsNull().Int8(); got.Valid {
		t.Fatalf("Int64(-3).NonPositiveIsNull().Int8() = %#v", got)
	}
	if got := pgencode.Int64(-3).ZeroIsNull().Int8(); !got.Valid || got.Int64 != -3 {
		t.Fatalf("Int64(-3).ZeroIsNull().Int8() = %#v (negatives stay valid under ZeroIsNull)", got)
	}

	// Every integer constructor exposes NonPositiveIsNull with the same semantics.
	if got := pgencode.Int8(-1).NonPositiveIsNull().Int8(); got.Valid {
		t.Fatalf("Int8(-1).NonPositiveIsNull().Int8() = %#v", got)
	}
	if got := pgencode.Int8(2).NonPositiveIsNull().Int8(); !got.Valid || got.Int64 != 2 {
		t.Fatalf("Int8(2).NonPositiveIsNull().Int8() = %#v", got)
	}
	if got := pgencode.Int16(0).NonPositiveIsNull().Int4(); got.Valid {
		t.Fatalf("Int16(0).NonPositiveIsNull().Int4() = %#v", got)
	}
	if got := pgencode.Int32(-5).NonPositiveIsNull().Int4(); got.Valid {
		t.Fatalf("Int32(-5).NonPositiveIsNull().Int4() = %#v", got)
	}
	if got := pgencode.Int(4).NonPositiveIsNull().Int8(); !got.Valid || got.Int64 != 4 {
		t.Fatalf("Int(4).NonPositiveIsNull().Int8() = %#v", got)
	}

	// Composes with the checked (Try*) outputs.
	if got, err := pgencode.Int64(-2).NonPositiveIsNull().TryInt4(); err != nil || got.Valid {
		t.Fatalf("Int64(-2).NonPositiveIsNull().TryInt4() = %#v, err=%v", got, err)
	}
	if got, err := pgencode.Int64(7).NonPositiveIsNull().TryInt2(); err != nil || !got.Valid || got.Int16 != 7 {
		t.Fatalf("Int64(7).NonPositiveIsNull().TryInt2() = %#v, err=%v", got, err)
	}

	// Float variant mirrors the integer semantics.
	if got := pgencode.Float64(-1.5).NonPositiveIsNull().Float8(); got.Valid {
		t.Fatalf("Float64(-1.5).NonPositiveIsNull().Float8() = %#v", got)
	}
	if got := pgencode.Float64(0).NonPositiveIsNull().Float8(); got.Valid {
		t.Fatalf("Float64(0).NonPositiveIsNull().Float8() = %#v", got)
	}
	if got := pgencode.Float64(2.5).NonPositiveIsNull().Float8(); !got.Valid || got.Float64 != 2.5 {
		t.Fatalf("Float64(2.5).NonPositiveIsNull().Float8() = %#v", got)
	}
}

func TestBool(t *testing.T) {
	if got := pgencode.Bool(true).Bool(); !got.Valid || !got.Bool {
		t.Fatalf("Bool().Bool() = %#v", got)
	}
	if got := pgencode.BoolPtr((*bool)(nil)).Bool(); got.Valid {
		t.Fatalf("BoolPtr(nil).Bool() = %#v", got)
	}

	value := true
	if got := pgencode.BoolPtr(&value).Bool(); !got.Valid || !got.Bool {
		t.Fatalf("BoolPtr().Bool() = %#v", got)
	}
}

func TestIntegers(t *testing.T) {
	if got := pgencode.Int8(7).Int2(); !got.Valid || got.Int16 != 7 {
		t.Fatalf("Int8().Int2() = %#v", got)
	}
	if got := pgencode.Int16(8).Int4(); !got.Valid || got.Int32 != 8 {
		t.Fatalf("Int16().Int4() = %#v", got)
	}
	if got := pgencode.Int32(9).Int4(); !got.Valid || got.Int32 != 9 {
		t.Fatalf("Int32().Int4() = %#v", got)
	}
	if got := pgencode.Int32(1 << 15).Int2(); !got.Valid || got.Int16 != math.MinInt16 {
		t.Fatalf("Int32(truncate).Int2() = %#v", got)
	}
	if got := pgencode.Int32(9).Int8(); !got.Valid || got.Int64 != 9 {
		t.Fatalf("Int32().Int8() = %#v", got)
	}
	if got := pgencode.Int64(10).Int8(); !got.Valid || got.Int64 != 10 {
		t.Fatalf("Int64().Int8() = %#v", got)
	}
	if got := pgencode.Int64(1 << 31).Int4(); !got.Valid || got.Int32 != math.MinInt32 {
		t.Fatalf("Int64(truncate).Int4() = %#v", got)
	}
	if got := pgencode.Int(11).Int8(); !got.Valid || got.Int64 != 11 {
		t.Fatalf("Int().Int8() = %#v", got)
	}
	if got := pgencode.IntPtr((*int)(nil)).Int8(); got.Valid {
		t.Fatalf("IntPtr(nil).Int8() = %#v", got)
	}

	if _, err := pgencode.Int32(1 << 20).TryInt2(); err == nil {
		t.Fatal("Int32().TryInt2() expected overflow error")
	}
	if _, err := pgencode.Int64(1 << 40).TryInt4(); err == nil {
		t.Fatal("Int64().TryInt4() expected overflow error")
	}
}

func TestIntegersZeroIsNull(t *testing.T) {
	// Non-zero values stay valid and carry the value through every output width.
	if got := pgencode.Int64(5).ZeroIsNull().Int8(); !got.Valid || got.Int64 != 5 {
		t.Fatalf("Int64(5).ZeroIsNull().Int8() = %#v", got)
	}
	if got := pgencode.Int64(5).ZeroIsNull().Int4(); !got.Valid || got.Int32 != 5 {
		t.Fatalf("Int64(5).ZeroIsNull().Int4() = %#v", got)
	}
	if got := pgencode.Int64(5).ZeroIsNull().Int2(); !got.Valid || got.Int16 != 5 {
		t.Fatalf("Int64(5).ZeroIsNull().Int2() = %#v", got)
	}

	// Zero values become NULL at every output width.
	if got := pgencode.Int64(0).ZeroIsNull().Int8(); got.Valid {
		t.Fatalf("Int64(0).ZeroIsNull().Int8() = %#v", got)
	}
	if got := pgencode.Int64(0).ZeroIsNull().Int4(); got.Valid {
		t.Fatalf("Int64(0).ZeroIsNull().Int4() = %#v", got)
	}
	if got := pgencode.Int64(0).ZeroIsNull().Int2(); got.Valid {
		t.Fatalf("Int64(0).ZeroIsNull().Int2() = %#v", got)
	}

	// Without ZeroIsNull, zero remains a valid value.
	if got := pgencode.Int64(0).Int8(); !got.Valid || got.Int64 != 0 {
		t.Fatalf("Int64(0).Int8() = %#v", got)
	}

	// Every integer constructor exposes ZeroIsNull with the same semantics.
	if got := pgencode.Int8(0).ZeroIsNull().Int8(); got.Valid {
		t.Fatalf("Int8(0).ZeroIsNull().Int8() = %#v", got)
	}
	if got := pgencode.Int8(3).ZeroIsNull().Int8(); !got.Valid || got.Int64 != 3 {
		t.Fatalf("Int8(3).ZeroIsNull().Int8() = %#v", got)
	}
	if got := pgencode.Int16(0).ZeroIsNull().Int4(); got.Valid {
		t.Fatalf("Int16(0).ZeroIsNull().Int4() = %#v", got)
	}
	if got := pgencode.Int16(4).ZeroIsNull().Int4(); !got.Valid || got.Int32 != 4 {
		t.Fatalf("Int16(4).ZeroIsNull().Int4() = %#v", got)
	}
	if got := pgencode.Int32(0).ZeroIsNull().Int4(); got.Valid {
		t.Fatalf("Int32(0).ZeroIsNull().Int4() = %#v", got)
	}
	if got := pgencode.Int(0).ZeroIsNull().Int8(); got.Valid {
		t.Fatalf("Int(0).ZeroIsNull().Int8() = %#v", got)
	}
	if got := pgencode.Int(6).ZeroIsNull().Int8(); !got.Valid || got.Int64 != 6 {
		t.Fatalf("Int(6).ZeroIsNull().Int8() = %#v", got)
	}

	// ZeroIsNull composes with the checked (Try*) outputs: zero -> NULL, no error.
	if got, err := pgencode.Int32(0).ZeroIsNull().TryInt2(); err != nil || got.Valid {
		t.Fatalf("Int32(0).ZeroIsNull().TryInt2() = %#v, err=%v", got, err)
	}
	if got, err := pgencode.Int64(7).ZeroIsNull().TryInt4(); err != nil || !got.Valid || got.Int32 != 7 {
		t.Fatalf("Int64(7).ZeroIsNull().TryInt4() = %#v, err=%v", got, err)
	}
}

func TestFloat64(t *testing.T) {
	if got := pgencode.Float64(1.5).Float8(); !got.Valid || got.Float64 != 1.5 {
		t.Fatalf("Float64().Float8() = %#v", got)
	}
	if got := pgencode.Float64Ptr((*float64)(nil)).Float8(); got.Valid {
		t.Fatalf("Float64Ptr(nil).Float8() = %#v", got)
	}
	if got := pgencode.Float64(0).ZeroIsNull().Float8(); got.Valid {
		t.Fatalf("Float64(0).ZeroIsNull().Float8() = %#v", got)
	}
	if got := pgencode.Float64(2.5).ZeroIsNull().Float8(); !got.Valid || got.Float64 != 2.5 {
		t.Fatalf("Float64(2.5).ZeroIsNull().Float8() = %#v", got)
	}
	if got := pgencode.Float64(0).Float8(); !got.Valid || got.Float64 != 0 {
		t.Fatalf("Float64(0).Float8() = %#v", got)
	}
}

func TestTime(t *testing.T) {
	now := time.Date(2026, 4, 1, 12, 34, 56, 0, time.UTC)
	zero := time.Time{}

	if got := pgencode.Time(now).Date(); !got.Valid || !got.Time.Equal(now) {
		t.Fatalf("Time().Date() = %#v", got)
	}
	if got := pgencode.Time(now).Timestamp(); !got.Valid || !got.Time.Equal(now) {
		t.Fatalf("Time().Timestamp() = %#v", got)
	}
	if got := pgencode.Time(now).Timestamptz(); !got.Valid || !got.Time.Equal(now) {
		t.Fatalf("Time().Timestamptz() = %#v", got)
	}
	if got := pgencode.Time(zero).ZeroIsNull().Timestamp(); got.Valid {
		t.Fatalf("Time(zero).ZeroIsNull().Timestamp() = %#v", got)
	}
	if got := pgencode.TimePtr((*time.Time)(nil)).Date(); got.Valid {
		t.Fatalf("TimePtr(nil).Date() = %#v", got)
	}
}

func TestUUID(t *testing.T) {
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	if got := pgencode.UUID(id).UUID(); !got.Valid || uuid.UUID(got.Bytes) != id {
		t.Fatalf("UUID().UUID() = %#v", got)
	}
	if got := pgencode.UUID(uuid.Nil).NilIsNull().UUID(); got.Valid {
		t.Fatalf("UUID(nil).NilIsNull().UUID() = %#v", got)
	}
	if got := pgencode.UUIDPtr((*uuid.UUID)(nil)).UUID(); got.Valid {
		t.Fatalf("UUIDPtr(nil).UUID() = %#v", got)
	}
}
