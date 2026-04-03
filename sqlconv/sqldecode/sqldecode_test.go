package sqldecode_test

import (
	"database/sql"
	"math"
	"math/bits"
	"testing"
	"time"

	"github.com/omniaura/go-kit/sqlconv/sqldecode"
)

func TestString(t *testing.T) {
	builder := sqldecode.String(sql.NullString{String: "hello", Valid: true})
	if got := builder.Value(); got != "hello" {
		t.Fatalf("String.Value() = %q", got)
	}
	if ptr := builder.Ptr(); ptr == nil || *ptr != "hello" {
		t.Fatalf("String.Ptr() = %#v", ptr)
	}

	filled := "seed"
	builder.Fill(&filled)
	if filled != "hello" {
		t.Fatalf("String.Fill() = %q", filled)
	}

	nullBuilder := sqldecode.String(sql.NullString{})
	if got := nullBuilder.Value(); got != "" {
		t.Fatalf("String(NULL).Value() = %q", got)
	}
	if ptr := nullBuilder.Ptr(); ptr != nil {
		t.Fatalf("String(NULL).Ptr() = %#v", ptr)
	}
	nullBuilder.Fill(&filled)
	if filled != "hello" {
		t.Fatalf("String(NULL).Fill() changed value to %q", filled)
	}

	fallbackBuilder := sqldecode.String(sql.NullString{}).Fallback("fallback")
	if got := fallbackBuilder.Value(); got != "fallback" {
		t.Fatalf("String(NULL).Fallback().Value() = %q", got)
	}
	if ptr := fallbackBuilder.Ptr(); ptr == nil || *ptr != "fallback" {
		t.Fatalf("String(NULL).Fallback().Ptr() = %#v", ptr)
	}
}

func TestBoolAndIntegers(t *testing.T) {
	if got := sqldecode.Bool(sql.NullBool{Bool: true, Valid: true}).Value(); !got {
		t.Fatalf("Bool.Value() = %v", got)
	}
	boolFilled := false
	sqldecode.Bool(sql.NullBool{}).Fallback(true).Fill(&boolFilled)
	if !boolFilled {
		t.Fatalf("Bool.Fallback().Fill() = %v", boolFilled)
	}

	if got := sqldecode.Int16(sql.NullInt16{Int16: 7, Valid: true}).Value(); got != 7 {
		t.Fatalf("Int16.Value() = %d", got)
	}
	intValue := sqldecode.Int16(sql.NullInt16{Int16: 8, Valid: true}).Int().Value()
	if intValue != 8 {
		t.Fatalf("Int16.Int().Value() = %d", intValue)
	}

	if got := sqldecode.Int32(sql.NullInt32{Int32: 17, Valid: true}).Value(); got != 17 {
		t.Fatalf("Int32.Value() = %d", got)
	}
	intValue = sqldecode.Int32(sql.NullInt32{Int32: 18, Valid: true}).Int().Value()
	if intValue != 18 {
		t.Fatalf("Int32.Int().Value() = %d", intValue)
	}

	if got := sqldecode.Int64(sql.NullInt64{Int64: 19, Valid: true}).Value(); got != 19 {
		t.Fatalf("Int64.Value() = %d", got)
	}
	ptr := sqldecode.Int64(sql.NullInt64{Int64: 20, Valid: true}).Int().Ptr()
	if ptr == nil || *ptr != 20 {
		t.Fatalf("Int64.Int().Ptr() = %#v", ptr)
	}
	tryPtr, err := sqldecode.Int64(sql.NullInt64{Int64: 20, Valid: true}).TryInt().Ptr()
	if err != nil || tryPtr == nil || *tryPtr != 20 {
		t.Fatalf("Int64.TryInt().Ptr() = %#v, %v", tryPtr, err)
	}

	if bits.UintSize == 32 {
		tooLarge := int64(math.MaxInt32) + 1
		if got := sqldecode.Int64(sql.NullInt64{Int64: tooLarge, Valid: true}).Int().Value(); got != math.MinInt32 {
			t.Fatalf("Int64(truncate).Int().Value() = %d", got)
		}
		if _, err := sqldecode.Int64(sql.NullInt64{Int64: tooLarge, Valid: true}).TryInt().Value(); err == nil {
			t.Fatal("Int64.TryInt().Value() expected overflow error on 32-bit")
		}
	}
}

func TestFloatAndTime(t *testing.T) {
	if got := sqldecode.Float64(sql.NullFloat64{Float64: 2.5, Valid: true}).Value(); got != 2.5 {
		t.Fatalf("Float64.Value() = %v", got)
	}
	floatFilled := 0.0
	sqldecode.Float64(sql.NullFloat64{}).Fallback(3.5).Fill(&floatFilled)
	if floatFilled != 3.5 {
		t.Fatalf("Float64.Fallback().Fill() = %v", floatFilled)
	}

	now := time.Date(2026, 4, 1, 12, 34, 56, 0, time.UTC)
	fallback := now.Add(-time.Hour)

	if got := sqldecode.Time(sql.NullTime{Time: now, Valid: true}).Value(); !got.Equal(now) {
		t.Fatalf("Time.Value() = %v", got)
	}
	timeFilled := time.Time{}
	sqldecode.Time(sql.NullTime{}).Fallback(fallback).Fill(&timeFilled)
	if !timeFilled.Equal(fallback) {
		t.Fatalf("Time.Fallback().Fill() = %v", timeFilled)
	}

	if ptr := sqldecode.Time(sql.NullTime{}).Ptr(); ptr != nil {
		t.Fatalf("Time(NULL).Ptr() = %#v", ptr)
	}
}
