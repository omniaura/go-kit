package sqlencode_test

import (
	"math"
	"testing"
	"time"

	"github.com/omniaura/go-kit/sqlconv/sqlencode"
)

func TestString(t *testing.T) {
	if got := sqlencode.String("hello").String(); !got.Valid || got.String != "hello" {
		t.Fatalf("String().String() = %#v", got)
	}
	if got := sqlencode.String("").String(); !got.Valid || got.String != "" {
		t.Fatalf("String(empty).String() = %#v", got)
	}
	if got := sqlencode.String("").EmptyIsNull().String(); got.Valid {
		t.Fatalf("String(empty).EmptyIsNull().String() = %#v", got)
	}
	if got := sqlencode.StringPtr((*string)(nil)).String(); got.Valid {
		t.Fatalf("StringPtr(nil).String() = %#v", got)
	}

	value := "world"
	if got := sqlencode.StringPtr(&value).String(); !got.Valid || got.String != "world" {
		t.Fatalf("StringPtr().String() = %#v", got)
	}
}

func TestBool(t *testing.T) {
	if got := sqlencode.Bool(true).Bool(); !got.Valid || !got.Bool {
		t.Fatalf("Bool().Bool() = %#v", got)
	}
	if got := sqlencode.BoolPtr((*bool)(nil)).Bool(); got.Valid {
		t.Fatalf("BoolPtr(nil).Bool() = %#v", got)
	}

	value := true
	if got := sqlencode.BoolPtr(&value).Bool(); !got.Valid || !got.Bool {
		t.Fatalf("BoolPtr().Bool() = %#v", got)
	}
}

func TestIntegers(t *testing.T) {
	if got := sqlencode.Int8(7).Int16(); !got.Valid || got.Int16 != 7 {
		t.Fatalf("Int8().Int16() = %#v", got)
	}
	if got := sqlencode.Int16(8).Int32(); !got.Valid || got.Int32 != 8 {
		t.Fatalf("Int16().Int32() = %#v", got)
	}
	if got := sqlencode.Int32(9).Int32(); !got.Valid || got.Int32 != 9 {
		t.Fatalf("Int32().Int32() = %#v", got)
	}
	if got := sqlencode.Int32(1 << 15).Int16(); !got.Valid || got.Int16 != math.MinInt16 {
		t.Fatalf("Int32(truncate).Int16() = %#v", got)
	}
	if got := sqlencode.Int32(9).Int64(); !got.Valid || got.Int64 != 9 {
		t.Fatalf("Int32().Int64() = %#v", got)
	}
	if got := sqlencode.Int64(10).Int64(); !got.Valid || got.Int64 != 10 {
		t.Fatalf("Int64().Int64() = %#v", got)
	}
	if got := sqlencode.Int64(1 << 31).Int32(); !got.Valid || got.Int32 != math.MinInt32 {
		t.Fatalf("Int64(truncate).Int32() = %#v", got)
	}
	if got := sqlencode.Int(11).Int64(); !got.Valid || got.Int64 != 11 {
		t.Fatalf("Int().Int64() = %#v", got)
	}
	if got := sqlencode.IntPtr((*int)(nil)).Int64(); got.Valid {
		t.Fatalf("IntPtr(nil).Int64() = %#v", got)
	}

	if _, err := sqlencode.Int32(1 << 20).TryInt16(); err == nil {
		t.Fatal("Int32().TryInt16() expected overflow error")
	}
	if _, err := sqlencode.Int64(1 << 40).TryInt32(); err == nil {
		t.Fatal("Int64().TryInt32() expected overflow error")
	}
}

func TestFloat64(t *testing.T) {
	if got := sqlencode.Float64(1.5).Float64(); !got.Valid || got.Float64 != 1.5 {
		t.Fatalf("Float64().Float64() = %#v", got)
	}
	if got := sqlencode.Float64Ptr((*float64)(nil)).Float64(); got.Valid {
		t.Fatalf("Float64Ptr(nil).Float64() = %#v", got)
	}
}

func TestTime(t *testing.T) {
	now := time.Date(2026, 4, 1, 12, 34, 56, 0, time.UTC)
	zero := time.Time{}

	if got := sqlencode.Time(now).Time(); !got.Valid || !got.Time.Equal(now) {
		t.Fatalf("Time().Time() = %#v", got)
	}
	if got := sqlencode.Time(zero).ZeroIsNull().Time(); got.Valid {
		t.Fatalf("Time(zero).ZeroIsNull().Time() = %#v", got)
	}
	if got := sqlencode.TimePtr((*time.Time)(nil)).Time(); got.Valid {
		t.Fatalf("TimePtr(nil).Time() = %#v", got)
	}
}
