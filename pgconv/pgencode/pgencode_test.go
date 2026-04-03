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

func TestFloat64(t *testing.T) {
	if got := pgencode.Float64(1.5).Float8(); !got.Valid || got.Float64 != 1.5 {
		t.Fatalf("Float64().Float8() = %#v", got)
	}
	if got := pgencode.Float64Ptr((*float64)(nil)).Float8(); got.Valid {
		t.Fatalf("Float64Ptr(nil).Float8() = %#v", got)
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
