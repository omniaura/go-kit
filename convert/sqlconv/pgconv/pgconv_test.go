package pgconv_test

import (
	"math"
	"math/big"
	"math/bits"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/omniaura/go-kit/convert/sqlconv/pgconv"
)

func TestTextBuilderAndConstructors(t *testing.T) {
	empty := ""
	greeting := "hello"

	if got := pgconv.ValidText(empty); !got.Valid || got.String != "" {
		t.Fatalf("ValidText(empty) = %#v", got)
	}
	if got := pgconv.NText(empty); got.Valid {
		t.Fatalf("NText(empty) should be invalid: %#v", got)
	}
	if got := pgconv.TextFromPtr(&empty); !got.Valid || got.String != "" {
		t.Fatalf("TextFromPtr(empty) = %#v", got)
	}
	if got := pgconv.TextFromPtr((*string)(nil)); got.Valid {
		t.Fatalf("TextFromPtr(nil) should be invalid: %#v", got)
	}

	builder := pgconv.Text(pgtype.Text{String: greeting, Valid: true})
	if got := builder.Value(); got != greeting {
		t.Fatalf("Text.Value() = %q", got)
	}
	if ptr := builder.Ptr(); ptr == nil || *ptr != greeting {
		t.Fatalf("Text.Ptr() = %#v", ptr)
	}
	filled := "seed"
	builder.Fill(&filled)
	if filled != greeting {
		t.Fatalf("Text.Fill() = %q", filled)
	}

	nullBuilder := pgconv.Text(pgtype.Text{})
	if got := nullBuilder.Value(); got != "" {
		t.Fatalf("Text(NULL).Value() = %q", got)
	}
	if ptr := nullBuilder.Ptr(); ptr != nil {
		t.Fatalf("Text(NULL).Ptr() = %#v", ptr)
	}
	nullBuilder.Fill(&filled)
	if filled != greeting {
		t.Fatalf("Text(NULL).Fill() changed value to %q", filled)
	}

	fallbackBuilder := pgconv.Text(pgtype.Text{}).Fallback("fallback")
	if got := fallbackBuilder.Value(); got != "fallback" {
		t.Fatalf("Text(NULL).Fallback().Value() = %q", got)
	}
	if ptr := fallbackBuilder.Ptr(); ptr == nil || *ptr != "fallback" {
		t.Fatalf("Text(NULL).Fallback().Ptr() = %#v", ptr)
	}
	fallbackBuilder.Fill(&filled)
	if filled != "fallback" {
		t.Fatalf("Text(NULL).Fallback().Fill() = %q", filled)
	}
}

func TestBoolAndIntegerBuilders(t *testing.T) {
	truth := true
	if got := pgconv.BoolFromPtr(&truth); !got.Valid || !got.Bool {
		t.Fatalf("BoolFromPtr() = %#v", got)
	}
	if got := pgconv.Bool(pgtype.Bool{Bool: true, Valid: true}).Value(); !got {
		t.Fatalf("Bool.Value() = %v", got)
	}
	boolFilled := false
	pgconv.Bool(pgtype.Bool{}).Fallback(true).Fill(&boolFilled)
	if !boolFilled {
		t.Fatalf("Bool.Fallback().Fill() = %v", boolFilled)
	}

	int16Value := int16(42)
	if got := pgconv.Int2FromPtr(&int16Value); !got.Valid || got.Int16 != 42 {
		t.Fatalf("Int2FromPtr() = %#v", got)
	}
	if got := pgconv.Int2(pgtype.Int2{Int16: 7, Valid: true}).Value(); got != 7 {
		t.Fatalf("Int2.Value() = %d", got)
	}
	intValue, err := pgconv.Int2(pgtype.Int2{Int16: 8, Valid: true}).Int().Value()
	if err != nil || intValue != 8 {
		t.Fatalf("Int2.Int().Value() = %d, %v", intValue, err)
	}
	intFilled := 0
	if err := pgconv.Int2(pgtype.Int2{}).Int().Fallback(9).Fill(&intFilled); err != nil {
		t.Fatalf("Int2.Int().Fallback().Fill() error = %v", err)
	}
	if intFilled != 9 {
		t.Fatalf("Int2.Int().Fallback().Fill() = %d", intFilled)
	}

	int32Value := int32(43)
	if got := pgconv.Int4FromPtr(&int32Value); !got.Valid || got.Int32 != 43 {
		t.Fatalf("Int4FromPtr() = %#v", got)
	}
	if got := pgconv.Int4(pgtype.Int4{Int32: 17, Valid: true}).Value(); got != 17 {
		t.Fatalf("Int4.Value() = %d", got)
	}
	intValue, err = pgconv.Int4(pgtype.Int4{Int32: 18, Valid: true}).Int().Value()
	if err != nil || intValue != 18 {
		t.Fatalf("Int4.Int().Value() = %d, %v", intValue, err)
	}

	int64Value := int64(44)
	if got := pgconv.Int8FromPtr(&int64Value); !got.Valid || got.Int64 != 44 {
		t.Fatalf("Int8FromPtr() = %#v", got)
	}
	if got := pgconv.Int8(pgtype.Int8{Int64: 19, Valid: true}).Value(); got != 19 {
		t.Fatalf("Int8.Value() = %d", got)
	}
	ptr, err := pgconv.Int8(pgtype.Int8{Int64: 20, Valid: true}).Int().Ptr()
	if err != nil || ptr == nil || *ptr != 20 {
		t.Fatalf("Int8.Int().Ptr() = %#v, %v", ptr, err)
	}

	if bits.UintSize == 32 {
		tooLarge := int64(math.MaxInt32) + 1
		if _, err := pgconv.Int8(pgtype.Int8{Int64: tooLarge, Valid: true}).Int().Value(); err == nil {
			t.Fatal("Int8.Int().Value() expected overflow error on 32-bit")
		}
	}
}

func TestFloatAndTimeBuilders(t *testing.T) {
	floatValue := 1.5
	if got := pgconv.Float8FromPtr(&floatValue); !got.Valid || got.Float64 != 1.5 {
		t.Fatalf("Float8FromPtr() = %#v", got)
	}
	if got := pgconv.Float8(pgtype.Float8{Float64: 2.5, Valid: true}).Value(); got != 2.5 {
		t.Fatalf("Float8.Value() = %v", got)
	}
	floatFilled := 0.0
	pgconv.Float8(pgtype.Float8{}).Fallback(3.5).Fill(&floatFilled)
	if floatFilled != 3.5 {
		t.Fatalf("Float8.Fallback().Fill() = %v", floatFilled)
	}

	now := time.Date(2026, 4, 1, 12, 34, 56, 0, time.UTC)
	fallback := now.Add(-time.Hour)

	if got := pgconv.NDate(now); !got.Valid || !got.Time.Equal(now) {
		t.Fatalf("NDate() = %#v", got)
	}
	if got := pgconv.DateFromPtr(&now); !got.Valid || !got.Time.Equal(now) {
		t.Fatalf("DateFromPtr() = %#v", got)
	}
	if got := pgconv.Date(pgtype.Date{Time: now, Valid: true, InfinityModifier: pgtype.Finite}).Value(); !got.Equal(now) {
		t.Fatalf("Date.Value() = %v", got)
	}
	dateFilled := time.Time{}
	pgconv.Date(pgtype.Date{}).Fallback(fallback).Fill(&dateFilled)
	if !dateFilled.Equal(fallback) {
		t.Fatalf("Date.Fallback().Fill() = %v", dateFilled)
	}

	for _, modifier := range []pgtype.InfinityModifier{pgtype.Infinity, pgtype.NegativeInfinity} {
		dateBuilder := pgconv.Date(pgtype.Date{Time: now, Valid: true, InfinityModifier: modifier})
		if got := dateBuilder.Value(); !got.IsZero() {
			t.Fatalf("Date(%v).Value() = %v", modifier, got)
		}
		if ptr := dateBuilder.Ptr(); ptr != nil {
			t.Fatalf("Date(%v).Ptr() = %#v", modifier, ptr)
		}
		seed := now
		dateBuilder.Fill(&seed)
		if !seed.Equal(now) {
			t.Fatalf("Date(%v).Fill() changed value to %v", modifier, seed)
		}
		dateBuilder.Fallback(fallback).Fill(&seed)
		if !seed.Equal(fallback) {
			t.Fatalf("Date(%v).Fallback().Fill() = %v", modifier, seed)
		}
	}

	if got := pgconv.TimestampFromPtr(&now); !got.Valid || !got.Time.Equal(now) {
		t.Fatalf("TimestampFromPtr() = %#v", got)
	}
	if got := pgconv.Timestamp(pgtype.Timestamp{Time: now, Valid: true, InfinityModifier: pgtype.Finite}).Value(); !got.Equal(now) {
		t.Fatalf("Timestamp.Value() = %v", got)
	}
	for _, modifier := range []pgtype.InfinityModifier{pgtype.Infinity, pgtype.NegativeInfinity} {
		builder := pgconv.Timestamp(pgtype.Timestamp{Time: now, Valid: true, InfinityModifier: modifier}).Fallback(fallback)
		if got := builder.Value(); !got.Equal(fallback) {
			t.Fatalf("Timestamp(%v).Fallback().Value() = %v", modifier, got)
		}
	}

	if got := pgconv.TimestamptzFromPtr(&now); !got.Valid || !got.Time.Equal(now) {
		t.Fatalf("TimestamptzFromPtr() = %#v", got)
	}
	if got := pgconv.Timestamptz(pgtype.Timestamptz{Time: now, Valid: true, InfinityModifier: pgtype.Finite}).Value(); !got.Equal(now) {
		t.Fatalf("Timestamptz.Value() = %v", got)
	}
	for _, modifier := range []pgtype.InfinityModifier{pgtype.Infinity, pgtype.NegativeInfinity} {
		builder := pgconv.Timestamptz(pgtype.Timestamptz{Time: now, Valid: true, InfinityModifier: modifier})
		if ptr := builder.Ptr(); ptr != nil {
			t.Fatalf("Timestamptz(%v).Ptr() = %#v", modifier, ptr)
		}
		seed := time.Time{}
		builder.Fallback(fallback).Fill(&seed)
		if !seed.Equal(fallback) {
			t.Fatalf("Timestamptz(%v).Fallback().Fill() = %v", modifier, seed)
		}
	}
}

func TestUUIDBuilderAndConstructors(t *testing.T) {
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

	builder := pgconv.UUID(pgtype.UUID{Bytes: [16]byte(id), Valid: true})
	if got := builder.Value(); got != id {
		t.Fatalf("UUID.Value() = %v", got)
	}
	if got := builder.String(); got != id.String() {
		t.Fatalf("UUID.String() = %q", got)
	}
	if ptr := builder.Ptr(); ptr == nil || *ptr != id {
		t.Fatalf("UUID.Ptr() = %#v", ptr)
	}

	filled := uuid.Nil
	pgconv.UUID(pgtype.UUID{}).Fallback(fallback).Fill(&filled)
	if filled != fallback {
		t.Fatalf("UUID.Fallback().Fill() = %v", filled)
	}
}

func TestNumericBuilders(t *testing.T) {
	numericFloat := pgtype.Numeric{Int: big.NewInt(12345), Exp: -2, Valid: true}
	gotFloat, err := pgconv.Numeric(numericFloat).Float64().Value()
	if err != nil {
		t.Fatalf("Numeric.Float64().Value() error = %v", err)
	}
	if gotFloat != 123.45 {
		t.Fatalf("Numeric.Float64().Value() = %v", gotFloat)
	}

	floatPtr, err := pgconv.Numeric(numericFloat).Float64().Ptr()
	if err != nil || floatPtr == nil || *floatPtr != 123.45 {
		t.Fatalf("Numeric.Float64().Ptr() = %#v, %v", floatPtr, err)
	}

	floatFilled := 0.0
	if err := pgconv.Numeric(pgtype.Numeric{}).Float64().Fallback(9.5).Fill(&floatFilled); err != nil {
		t.Fatalf("Numeric.Float64().Fallback().Fill() error = %v", err)
	}
	if floatFilled != 9.5 {
		t.Fatalf("Numeric.Float64().Fallback().Fill() = %v", floatFilled)
	}

	numericInt := pgtype.Numeric{Int: big.NewInt(42), Exp: 0, Valid: true}
	gotInt, err := pgconv.Numeric(numericInt).Int64().Value()
	if err != nil {
		t.Fatalf("Numeric.Int64().Value() error = %v", err)
	}
	if gotInt != 42 {
		t.Fatalf("Numeric.Int64().Value() = %d", gotInt)
	}

	intFilled := int64(0)
	if err := pgconv.Numeric(pgtype.Numeric{}).Int64().Fallback(7).Fill(&intFilled); err != nil {
		t.Fatalf("Numeric.Int64().Fallback().Fill() error = %v", err)
	}
	if intFilled != 7 {
		t.Fatalf("Numeric.Int64().Fallback().Fill() = %d", intFilled)
	}

	plainInt := 0
	if err := pgconv.Numeric(numericInt).Int().Fill(&plainInt); err != nil {
		t.Fatalf("Numeric.Int().Fill() error = %v", err)
	}
	if plainInt != 42 {
		t.Fatalf("Numeric.Int().Fill() = %d", plainInt)
	}

	tooLargeFloat := pgtype.Numeric{Int: new(big.Int).Exp(big.NewInt(10), big.NewInt(400), nil), Exp: 0, Valid: true}
	floatSeed := 55.0
	if _, err := pgconv.Numeric(tooLargeFloat).Float64().Value(); err == nil {
		t.Fatal("Numeric.Float64().Value() expected error")
	}
	if err := pgconv.Numeric(tooLargeFloat).Float64().Fallback(8.5).Fill(&floatSeed); err == nil {
		t.Fatal("Numeric.Float64().Fallback().Fill() expected error")
	}
	if floatSeed != 55.0 {
		t.Fatalf("Numeric.Float64().Fallback().Fill() changed value to %v", floatSeed)
	}

	fractionalInt := pgtype.Numeric{Int: big.NewInt(15), Exp: -1, Valid: true}
	intSeed := int64(77)
	if _, err := pgconv.Numeric(fractionalInt).Int64().Value(); err == nil {
		t.Fatal("Numeric.Int64().Value() expected error")
	}
	if err := pgconv.Numeric(fractionalInt).Int64().Fallback(9).Fill(&intSeed); err == nil {
		t.Fatal("Numeric.Int64().Fallback().Fill() expected error")
	}
	if intSeed != 77 {
		t.Fatalf("Numeric.Int64().Fallback().Fill() changed value to %d", intSeed)
	}
}
