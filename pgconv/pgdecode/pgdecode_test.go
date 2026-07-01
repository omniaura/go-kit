package pgdecode_test

import (
	"math"
	"math/big"
	"math/bits"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/omniaura/go-kit/pgconv/pgdecode"
)

func TestText(t *testing.T) {
	builder := pgdecode.Text(pgtype.Text{String: "hello", Valid: true})
	if got := builder.Value(); got != "hello" {
		t.Fatalf("Text.Value() = %q", got)
	}
	if ptr := builder.Ptr(); ptr == nil || *ptr != "hello" {
		t.Fatalf("Text.Ptr() = %#v", ptr)
	}

	filled := "seed"
	builder.Fill(&filled)
	if filled != "hello" {
		t.Fatalf("Text.Fill() = %q", filled)
	}

	nullBuilder := pgdecode.Text(pgtype.Text{})
	if got := nullBuilder.Value(); got != "" {
		t.Fatalf("Text(NULL).Value() = %q", got)
	}
	if ptr := nullBuilder.Ptr(); ptr != nil {
		t.Fatalf("Text(NULL).Ptr() = %#v", ptr)
	}
	nullBuilder.Fill(&filled)
	if filled != "hello" {
		t.Fatalf("Text(NULL).Fill() changed value to %q", filled)
	}

	fallbackBuilder := pgdecode.Text(pgtype.Text{}).Fallback("fallback")
	if got := fallbackBuilder.Value(); got != "fallback" {
		t.Fatalf("Text(NULL).Fallback().Value() = %q", got)
	}
	if ptr := fallbackBuilder.Ptr(); ptr == nil || *ptr != "fallback" {
		t.Fatalf("Text(NULL).Fallback().Ptr() = %#v", ptr)
	}
}

func TestCoalesceText(t *testing.T) {
	title := pgtype.Text{String: "title", Valid: true}
	description := pgtype.Text{String: "description", Valid: true}
	emptyValid := pgtype.Text{String: "", Valid: true}
	null := pgtype.Text{}

	// First non-empty wins: a non-empty description is preferred over title.
	if got := pgdecode.CoalesceText(description, title).Value(); got != "description" {
		t.Fatalf("CoalesceText(description, title).Value() = %q", got)
	}
	// Empty (even if Valid) is skipped, falling through to the next non-empty.
	if got := pgdecode.CoalesceText(emptyValid, title).Value(); got != "title" {
		t.Fatalf("CoalesceText(emptyValid, title).Value() = %q", got)
	}
	// NULL is skipped too.
	if got := pgdecode.CoalesceText(null, title).Value(); got != "title" {
		t.Fatalf("CoalesceText(null, title).Value() = %q", got)
	}

	// All-empty resolves to "" and remains non-NULL (Ptr/Fill still yield "").
	allEmpty := pgdecode.CoalesceText(null, emptyValid)
	if got := allEmpty.Value(); got != "" {
		t.Fatalf("CoalesceText(all empty).Value() = %q", got)
	}
	if ptr := allEmpty.Ptr(); ptr == nil || *ptr != "" {
		t.Fatalf("CoalesceText(all empty).Ptr() = %#v", ptr)
	}
	filled := "seed"
	allEmpty.Fill(&filled)
	if filled != "" {
		t.Fatalf("CoalesceText(all empty).Fill() = %q", filled)
	}

	// No args behaves like all-empty.
	if got := pgdecode.CoalesceText().Value(); got != "" {
		t.Fatalf("CoalesceText().Value() = %q", got)
	}

	// Ptr/Fill on a match.
	if ptr := pgdecode.CoalesceText(description, title).Ptr(); ptr == nil || *ptr != "description" {
		t.Fatalf("CoalesceText(description, title).Ptr() = %#v", ptr)
	}
	dst := "seed"
	pgdecode.CoalesceText(null, title).Fill(&dst)
	if dst != "title" {
		t.Fatalf("CoalesceText(null, title).Fill() = %q", dst)
	}
}

func TestBoolAndIntegers(t *testing.T) {
	if got := pgdecode.Bool(pgtype.Bool{Bool: true, Valid: true}).Value(); !got {
		t.Fatalf("Bool.Value() = %v", got)
	}
	boolFilled := false
	pgdecode.Bool(pgtype.Bool{}).Fallback(true).Fill(&boolFilled)
	if !boolFilled {
		t.Fatalf("Bool.Fallback().Fill() = %v", boolFilled)
	}

	if got := pgdecode.Int2(pgtype.Int2{Int16: 7, Valid: true}).Value(); got != 7 {
		t.Fatalf("Int2.Value() = %d", got)
	}
	intValue := pgdecode.Int2(pgtype.Int2{Int16: 8, Valid: true}).Int().Value()
	if intValue != 8 {
		t.Fatalf("Int2.Int().Value() = %d", intValue)
	}

	if got := pgdecode.Int4(pgtype.Int4{Int32: 17, Valid: true}).Value(); got != 17 {
		t.Fatalf("Int4.Value() = %d", got)
	}
	intValue = pgdecode.Int4(pgtype.Int4{Int32: 18, Valid: true}).Int().Value()
	if intValue != 18 {
		t.Fatalf("Int4.Int().Value() = %d", intValue)
	}

	if got := pgdecode.Int8(pgtype.Int8{Int64: 19, Valid: true}).Value(); got != 19 {
		t.Fatalf("Int8.Value() = %d", got)
	}
	ptr := pgdecode.Int8(pgtype.Int8{Int64: 20, Valid: true}).Int().Ptr()
	if ptr == nil || *ptr != 20 {
		t.Fatalf("Int8.Int().Ptr() = %#v", ptr)
	}
	tryPtr, err := pgdecode.Int8(pgtype.Int8{Int64: 20, Valid: true}).TryInt().Ptr()
	if err != nil || tryPtr == nil || *tryPtr != 20 {
		t.Fatalf("Int8.TryInt().Ptr() = %#v, %v", tryPtr, err)
	}

	if bits.UintSize == 32 {
		tooLarge := int64(math.MaxInt32) + 1
		if got := pgdecode.Int8(pgtype.Int8{Int64: tooLarge, Valid: true}).Int().Value(); got != math.MinInt32 {
			t.Fatalf("Int8(truncate).Int().Value() = %d", got)
		}
		if _, err := pgdecode.Int8(pgtype.Int8{Int64: tooLarge, Valid: true}).TryInt().Value(); err == nil {
			t.Fatal("Int8.TryInt().Value() expected overflow error on 32-bit")
		}
	}
}

func TestFloatAndTime(t *testing.T) {
	if got := pgdecode.Float8(pgtype.Float8{Float64: 2.5, Valid: true}).Value(); got != 2.5 {
		t.Fatalf("Float8.Value() = %v", got)
	}
	floatFilled := 0.0
	pgdecode.Float8(pgtype.Float8{}).Fallback(3.5).Fill(&floatFilled)
	if floatFilled != 3.5 {
		t.Fatalf("Float8.Fallback().Fill() = %v", floatFilled)
	}

	now := time.Date(2026, 4, 1, 12, 34, 56, 0, time.UTC)
	fallback := now.Add(-time.Hour)

	if got := pgdecode.Date(pgtype.Date{Time: now, Valid: true, InfinityModifier: pgtype.Finite}).Value(); !got.Equal(now) {
		t.Fatalf("Date.Value() = %v", got)
	}
	dateFilled := time.Time{}
	pgdecode.Date(pgtype.Date{}).Fallback(fallback).Fill(&dateFilled)
	if !dateFilled.Equal(fallback) {
		t.Fatalf("Date.Fallback().Fill() = %v", dateFilled)
	}

	for _, modifier := range []pgtype.InfinityModifier{pgtype.Infinity, pgtype.NegativeInfinity} {
		dateBuilder := pgdecode.Date(pgtype.Date{Time: now, Valid: true, InfinityModifier: modifier})
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

	if got := pgdecode.Timestamp(pgtype.Timestamp{Time: now, Valid: true, InfinityModifier: pgtype.Finite}).Value(); !got.Equal(now) {
		t.Fatalf("Timestamp.Value() = %v", got)
	}
	for _, modifier := range []pgtype.InfinityModifier{pgtype.Infinity, pgtype.NegativeInfinity} {
		builder := pgdecode.Timestamp(pgtype.Timestamp{Time: now, Valid: true, InfinityModifier: modifier}).Fallback(fallback)
		if got := builder.Value(); !got.Equal(fallback) {
			t.Fatalf("Timestamp(%v).Fallback().Value() = %v", modifier, got)
		}
	}

	if got := pgdecode.Timestamptz(pgtype.Timestamptz{Time: now, Valid: true, InfinityModifier: pgtype.Finite}).Value(); !got.Equal(now) {
		t.Fatalf("Timestamptz.Value() = %v", got)
	}
	for _, modifier := range []pgtype.InfinityModifier{pgtype.Infinity, pgtype.NegativeInfinity} {
		builder := pgdecode.Timestamptz(pgtype.Timestamptz{Time: now, Valid: true, InfinityModifier: modifier})
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

func TestUUID(t *testing.T) {
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	fallback := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	builder := pgdecode.UUID(pgtype.UUID{Bytes: [16]byte(id), Valid: true})
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
	pgdecode.UUID(pgtype.UUID{}).Fallback(fallback).Fill(&filled)
	if filled != fallback {
		t.Fatalf("UUID.Fallback().Fill() = %v", filled)
	}
}

func TestNumeric(t *testing.T) {
	numericFloat := pgtype.Numeric{Int: big.NewInt(12345), Exp: -2, Valid: true}
	gotFloat, err := pgdecode.Numeric(numericFloat).Float64().Value()
	if err != nil {
		t.Fatalf("Numeric.Float64().Value() error = %v", err)
	}
	if gotFloat != 123.45 {
		t.Fatalf("Numeric.Float64().Value() = %v", gotFloat)
	}

	floatPtr, err := pgdecode.Numeric(numericFloat).Float64().Ptr()
	if err != nil || floatPtr == nil || *floatPtr != 123.45 {
		t.Fatalf("Numeric.Float64().Ptr() = %#v, %v", floatPtr, err)
	}

	floatFilled := 0.0
	if err := pgdecode.Numeric(pgtype.Numeric{}).Float64().Fallback(9.5).Fill(&floatFilled); err != nil {
		t.Fatalf("Numeric.Float64().Fallback().Fill() error = %v", err)
	}
	if floatFilled != 9.5 {
		t.Fatalf("Numeric.Float64().Fallback().Fill() = %v", floatFilled)
	}

	numericInt := pgtype.Numeric{Int: big.NewInt(42), Exp: 0, Valid: true}
	gotInt, err := pgdecode.Numeric(numericInt).Int64().Value()
	if err != nil {
		t.Fatalf("Numeric.Int64().Value() error = %v", err)
	}
	if gotInt != 42 {
		t.Fatalf("Numeric.Int64().Value() = %d", gotInt)
	}

	intFilled := int64(0)
	if err := pgdecode.Numeric(pgtype.Numeric{}).Int64().Fallback(7).Fill(&intFilled); err != nil {
		t.Fatalf("Numeric.Int64().Fallback().Fill() error = %v", err)
	}
	if intFilled != 7 {
		t.Fatalf("Numeric.Int64().Fallback().Fill() = %d", intFilled)
	}

	plainInt := 0
	if err := pgdecode.Numeric(numericInt).Int().Fill(&plainInt); err != nil {
		t.Fatalf("Numeric.Int().Fill() error = %v", err)
	}
	if plainInt != 42 {
		t.Fatalf("Numeric.Int().Fill() = %d", plainInt)
	}

	tooLargeFloat := pgtype.Numeric{Int: new(big.Int).Exp(big.NewInt(10), big.NewInt(400), nil), Exp: 0, Valid: true}
	floatSeed := 55.0
	if _, err := pgdecode.Numeric(tooLargeFloat).Float64().Value(); err == nil {
		t.Fatal("Numeric.Float64().Value() expected error")
	}
	if err := pgdecode.Numeric(tooLargeFloat).Float64().Fallback(8.5).Fill(&floatSeed); err == nil {
		t.Fatal("Numeric.Float64().Fallback().Fill() expected error")
	}
	if floatSeed != 55.0 {
		t.Fatalf("Numeric.Float64().Fallback().Fill() changed value to %v", floatSeed)
	}

	fractionalInt := pgtype.Numeric{Int: big.NewInt(15), Exp: -1, Valid: true}
	intSeed := int64(77)
	if _, err := pgdecode.Numeric(fractionalInt).Int64().Value(); err == nil {
		t.Fatal("Numeric.Int64().Value() expected error")
	}
	if err := pgdecode.Numeric(fractionalInt).Int64().Fallback(9).Fill(&intSeed); err == nil {
		t.Fatal("Numeric.Int64().Fallback().Fill() expected error")
	}
	if intSeed != 77 {
		t.Fatalf("Numeric.Int64().Fallback().Fill() changed value to %d", intSeed)
	}
}
