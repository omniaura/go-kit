package pgconv_test

import (
	"math/big"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/omniaura/go-kit/convert/sqlconv/pgconv"
)

type namedString string

type namedBool bool

type namedInt int

type namedFloat float64

func TestTextHelpers(t *testing.T) {
	value := namedString("hello")
	if got := pgconv.Text(value); got.String != "hello" || !got.Valid {
		t.Fatalf("Text() = %#v", got)
	}

	if got := pgconv.NText(namedString("")); got.Valid {
		t.Fatalf("NText(empty) should be invalid: %#v", got)
	}

	if got := pgconv.TextFromPtr(&value); got.String != "hello" || !got.Valid {
		t.Fatalf("TextFromPtr() = %#v", got)
	}
	if got := pgconv.TextFromPtr((*namedString)(nil)); got.Valid {
		t.Fatalf("TextFromPtr(nil) should be invalid: %#v", got)
	}

	text := pgtype.Text{String: "greet", Valid: true}
	if got := pgconv.TextValue[namedString](text); got != "greet" {
		t.Fatalf("TextValue() = %q", got)
	}
	if got := pgconv.TextOr(pgtype.Text{}, namedString("fallback")); got != "fallback" {
		t.Fatalf("TextOr() = %q", got)
	}

	ptr := pgconv.TextPtr(text)
	if ptr == nil || *ptr != "greet" {
		t.Fatalf("TextPtr() = %#v", ptr)
	}
	if got := pgconv.TextPtr(pgtype.Text{}); got != nil {
		t.Fatalf("TextPtr(invalid) = %#v", got)
	}

	filled := namedString("seed")
	pgconv.FillText(text, &filled)
	if filled != "greet" {
		t.Fatalf("FillText() = %q", filled)
	}
	pgconv.FillText(pgtype.Text{}, &filled)
	if filled != "greet" {
		t.Fatalf("FillText(invalid) changed value to %q", filled)
	}
	pgconv.FillTextOr(pgtype.Text{}, &filled, namedString("fallback"))
	if filled != "fallback" {
		t.Fatalf("FillTextOr() = %q", filled)
	}
}

func TestBoolAndNumberHelpers(t *testing.T) {
	b := namedBool(true)
	if got := pgconv.BoolFromPtr(&b); !got.Valid || !got.Bool {
		t.Fatalf("BoolFromPtr() = %#v", got)
	}
	if got := pgconv.BoolValue[namedBool](pgtype.Bool{Bool: true, Valid: true}); got != true {
		t.Fatalf("BoolValue() = %v", got)
	}
	if got := pgconv.BoolOr(pgtype.Bool{}, namedBool(true)); got != true {
		t.Fatalf("BoolOr() = %v", got)
	}
	if got := pgconv.BoolPtr(pgtype.Bool{Bool: true, Valid: true}); got == nil || *got != true {
		t.Fatalf("BoolPtr() = %#v", got)
	}
	filledBool := namedBool(false)
	pgconv.FillBoolOr(pgtype.Bool{}, &filledBool, namedBool(true))
	if filledBool != true {
		t.Fatalf("FillBoolOr() = %v", filledBool)
	}

	intVal := namedInt(42)
	if got := pgconv.NInt2(intVal); !got.Valid || got.Int16 != 42 {
		t.Fatalf("NInt2() = %#v", got)
	}
	if got := pgconv.Int2FromPtr(&intVal); !got.Valid || got.Int16 != 42 {
		t.Fatalf("Int2FromPtr() = %#v", got)
	}
	if got := pgconv.Int2Value[namedInt](pgtype.Int2{Int16: 7, Valid: true}); got != 7 {
		t.Fatalf("Int2Value() = %v", got)
	}

	if got := pgconv.NInt4(intVal); !got.Valid || got.Int32 != 42 {
		t.Fatalf("NInt4() = %#v", got)
	}
	if got := pgconv.Int4Or(pgtype.Int4{}, namedInt(9)); got != 9 {
		t.Fatalf("Int4Or() = %v", got)
	}
	ptr4 := pgconv.Int4Ptr[namedInt](pgtype.Int4{Int32: 11, Valid: true})
	if ptr4 == nil || *ptr4 != 11 {
		t.Fatalf("Int4Ptr() = %#v", ptr4)
	}
	filled4 := namedInt(0)
	pgconv.FillInt4(pgtype.Int4{Int32: 12, Valid: true}, &filled4)
	if filled4 != 12 {
		t.Fatalf("FillInt4() = %v", filled4)
	}

	if got := pgconv.NInt8(intVal); !got.Valid || got.Int64 != 42 {
		t.Fatalf("NInt8() = %#v", got)
	}
	if got := pgconv.Int8Or(pgtype.Int8{}, namedInt(13)); got != 13 {
		t.Fatalf("Int8Or() = %v", got)
	}
	filled8 := namedInt(0)
	pgconv.FillInt8Or(pgtype.Int8{}, &filled8, namedInt(14))
	if filled8 != 14 {
		t.Fatalf("FillInt8Or() = %v", filled8)
	}

	floatVal := namedFloat(1.5)
	if got := pgconv.NFloat8(floatVal); !got.Valid || got.Float64 != 1.5 {
		t.Fatalf("NFloat8() = %#v", got)
	}
	if got := pgconv.Float8FromPtr(&floatVal); !got.Valid || got.Float64 != 1.5 {
		t.Fatalf("Float8FromPtr() = %#v", got)
	}
	if got := pgconv.Float8Value[namedFloat](pgtype.Float8{Float64: 2.5, Valid: true}); got != 2.5 {
		t.Fatalf("Float8Value() = %v", got)
	}
	if got := pgconv.Float8Or(pgtype.Float8{}, namedFloat(3.5)); got != 3.5 {
		t.Fatalf("Float8Or() = %v", got)
	}
	ptr8 := pgconv.Float8Ptr[namedFloat](pgtype.Float8{Float64: 4.5, Valid: true})
	if ptr8 == nil || *ptr8 != 4.5 {
		t.Fatalf("Float8Ptr() = %#v", ptr8)
	}
	filledFloat := namedFloat(0)
	pgconv.FillFloat8Or(pgtype.Float8{}, &filledFloat, namedFloat(5.5))
	if filledFloat != 5.5 {
		t.Fatalf("FillFloat8Or() = %v", filledFloat)
	}
}

func TestTimeHelpers(t *testing.T) {
	now := time.Date(2026, 4, 1, 12, 34, 56, 0, time.UTC)
	fallback := now.Add(-time.Hour)

	if got := pgconv.NDate(now); !got.Valid || !got.Time.Equal(now) {
		t.Fatalf("NDate() = %#v", got)
	}
	if got := pgconv.NDate(time.Time{}); got.Valid {
		t.Fatalf("NDate(zero) should be invalid: %#v", got)
	}
	if got := pgconv.DateFromPtr(&now); !got.Valid || !got.Time.Equal(now) {
		t.Fatalf("DateFromPtr() = %#v", got)
	}
	if got := pgconv.DateValue(pgtype.Date{Time: now, Valid: true}); !got.Equal(now) {
		t.Fatalf("DateValue() = %v", got)
	}
	if got := pgconv.DateOr(pgtype.Date{}, fallback); !got.Equal(fallback) {
		t.Fatalf("DateOr() = %v", got)
	}
	if got := pgconv.DatePtr(pgtype.Date{Time: now, Valid: true}); got == nil || !got.Equal(now) {
		t.Fatalf("DatePtr() = %#v", got)
	}
	filledDate := time.Time{}
	pgconv.FillDateOr(pgtype.Date{}, &filledDate, fallback)
	if !filledDate.Equal(fallback) {
		t.Fatalf("FillDateOr() = %v", filledDate)
	}

	if got := pgconv.NTimestamp(now); !got.Valid || !got.Time.Equal(now) {
		t.Fatalf("NTimestamp() = %#v", got)
	}
	if got := pgconv.TimestampFromPtr(&now); !got.Valid || !got.Time.Equal(now) {
		t.Fatalf("TimestampFromPtr() = %#v", got)
	}
	if got := pgconv.TimestampValue(pgtype.Timestamp{Time: now, Valid: true}); !got.Equal(now) {
		t.Fatalf("TimestampValue() = %v", got)
	}
	if got := pgconv.TimestampOr(pgtype.Timestamp{}, fallback); !got.Equal(fallback) {
		t.Fatalf("TimestampOr() = %v", got)
	}
	if got := pgconv.TimestampPtr(pgtype.Timestamp{Time: now, Valid: true}); got == nil || !got.Equal(now) {
		t.Fatalf("TimestampPtr() = %#v", got)
	}
	filledTimestamp := time.Time{}
	pgconv.FillTimestampOr(pgtype.Timestamp{}, &filledTimestamp, fallback)
	if !filledTimestamp.Equal(fallback) {
		t.Fatalf("FillTimestampOr() = %v", filledTimestamp)
	}

	if got := pgconv.NTimestamptz(now); !got.Valid || !got.Time.Equal(now) {
		t.Fatalf("NTimestamptz() = %#v", got)
	}
	if got := pgconv.TimestamptzFromPtr(&now); !got.Valid || !got.Time.Equal(now) {
		t.Fatalf("TimestamptzFromPtr() = %#v", got)
	}
	if got := pgconv.TimestamptzValue(pgtype.Timestamptz{Time: now, Valid: true}); !got.Equal(now) {
		t.Fatalf("TimestamptzValue() = %v", got)
	}
	if got := pgconv.TimestamptzOr(pgtype.Timestamptz{}, fallback); !got.Equal(fallback) {
		t.Fatalf("TimestamptzOr() = %v", got)
	}
	if got := pgconv.TimestamptzPtr(pgtype.Timestamptz{Time: now, Valid: true}); got == nil || !got.Equal(now) {
		t.Fatalf("TimestamptzPtr() = %#v", got)
	}
	filledTimestamptz := time.Time{}
	pgconv.FillTimestamptzOr(pgtype.Timestamptz{}, &filledTimestamptz, fallback)
	if !filledTimestamptz.Equal(fallback) {
		t.Fatalf("FillTimestamptzOr() = %v", filledTimestamptz)
	}
}

func TestUUIDHelpers(t *testing.T) {
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	fallback := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	if got := pgconv.NUUID(id); !got.Valid || uuid.UUID(got.Bytes) != id {
		t.Fatalf("NUUID() = %#v", got)
	}
	if got := pgconv.NUUID(uuid.Nil); got.Valid {
		t.Fatalf("NUUID(nil) should be invalid: %#v", got)
	}
	if got := pgconv.UUIDFromPtr(&id); !got.Valid || uuid.UUID(got.Bytes) != id {
		t.Fatalf("UUIDFromPtr() = %#v", got)
	}
	if got := pgconv.UUIDValue(pgtype.UUID{Bytes: [16]byte(id), Valid: true}); got != id {
		t.Fatalf("UUIDValue() = %v", got)
	}
	if got := pgconv.UUIDOr(pgtype.UUID{}, fallback); got != fallback {
		t.Fatalf("UUIDOr() = %v", got)
	}
	if got := pgconv.UUIDString(pgtype.UUID{Bytes: [16]byte(id), Valid: true}); got != id.String() {
		t.Fatalf("UUIDString() = %q", got)
	}
	if got := pgconv.UUIDStringOr(pgtype.UUID{}, fallback.String()); got != fallback.String() {
		t.Fatalf("UUIDStringOr() = %q", got)
	}
	ptr := pgconv.UUIDPtr(pgtype.UUID{Bytes: [16]byte(id), Valid: true})
	if ptr == nil || *ptr != id {
		t.Fatalf("UUIDPtr() = %#v", ptr)
	}

	filled := uuid.Nil
	pgconv.FillUUID(pgtype.UUID{Bytes: [16]byte(id), Valid: true}, &filled)
	if filled != id {
		t.Fatalf("FillUUID() = %v", filled)
	}
	pgconv.FillUUIDOr(pgtype.UUID{}, &filled, fallback)
	if filled != fallback {
		t.Fatalf("FillUUIDOr() = %v", filled)
	}
}

func TestNumericHelpers(t *testing.T) {
	numericFloat := pgtype.Numeric{Int: big.NewInt(12345), Exp: -2, Valid: true}
	gotFloat, err := pgconv.NumericFloat64(numericFloat)
	if err != nil {
		t.Fatalf("NumericFloat64() error = %v", err)
	}
	if gotFloat != 123.45 {
		t.Fatalf("NumericFloat64() = %v", gotFloat)
	}

	ptrFloat, err := pgconv.NumericFloat64Ptr(numericFloat)
	if err != nil {
		t.Fatalf("NumericFloat64Ptr() error = %v", err)
	}
	if ptrFloat == nil || *ptrFloat != 123.45 {
		t.Fatalf("NumericFloat64Ptr() = %#v", ptrFloat)
	}

	filledFloat := 0.0
	if err := pgconv.FillNumericFloat64Or(pgtype.Numeric{}, &filledFloat, 9.9); err != nil {
		t.Fatalf("FillNumericFloat64Or() error = %v", err)
	}
	if filledFloat != 9.9 {
		t.Fatalf("FillNumericFloat64Or() = %v", filledFloat)
	}

	numericInt := pgtype.Numeric{Int: big.NewInt(77), Exp: 0, Valid: true}
	gotInt, err := pgconv.NumericInt64(numericInt)
	if err != nil {
		t.Fatalf("NumericInt64() error = %v", err)
	}
	if gotInt != 77 {
		t.Fatalf("NumericInt64() = %v", gotInt)
	}

	ptrInt, err := pgconv.NumericInt64Ptr(numericInt)
	if err != nil {
		t.Fatalf("NumericInt64Ptr() error = %v", err)
	}
	if ptrInt == nil || *ptrInt != 77 {
		t.Fatalf("NumericInt64Ptr() = %#v", ptrInt)
	}

	filledInt := int64(0)
	if err := pgconv.FillNumericInt64Or(pgtype.Numeric{}, &filledInt, 88); err != nil {
		t.Fatalf("FillNumericInt64Or() error = %v", err)
	}
	if filledInt != 88 {
		t.Fatalf("FillNumericInt64Or() = %v", filledInt)
	}

	fallbackFloat, err := pgconv.NumericFloat64Or(pgtype.Numeric{}, 1.25)
	if err != nil {
		t.Fatalf("NumericFloat64Or() error = %v", err)
	}
	if fallbackFloat != 1.25 {
		t.Fatalf("NumericFloat64Or() = %v", fallbackFloat)
	}

	fallbackInt, err := pgconv.NumericInt64Or(pgtype.Numeric{}, 99)
	if err != nil {
		t.Fatalf("NumericInt64Or() error = %v", err)
	}
	if fallbackInt != 99 {
		t.Fatalf("NumericInt64Or() = %v", fallbackInt)
	}
}
