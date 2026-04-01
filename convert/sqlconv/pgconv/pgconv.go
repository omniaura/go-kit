package pgconv

import (
	"fmt"
	"math"
	"math/bits"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	maxInt32 = 1<<31 - 1
	minInt32 = -1 << 31
)

type nullableValue[T any] struct {
	valid       bool
	value       T
	fallback    T
	hasFallback bool
}

func newNullableValue[T any](valid bool, value T) nullableValue[T] {
	return nullableValue[T]{valid: valid, value: value}
}

func (v nullableValue[T]) withFallback(fallback T) nullableValue[T] {
	v.fallback = fallback
	v.hasFallback = true
	return v
}

func (v nullableValue[T]) resolve(zero T) (T, bool) {
	if v.valid {
		return v.value, true
	}
	if v.hasFallback {
		return v.fallback, true
	}
	return zero, false
}

func ptrOf[T any](value T) *T {
	v := value
	return &v
}

func finiteDate(value pgtype.Date) bool {
	return value.Valid && value.InfinityModifier == pgtype.Finite
}

func finiteTimestamp(value pgtype.Timestamp) bool {
	return value.Valid && value.InfinityModifier == pgtype.Finite
}

func finiteTimestamptz(value pgtype.Timestamptz) bool {
	return value.Valid && value.InfinityModifier == pgtype.Finite
}

func fitsInt64InInt(value int64) bool {
	if bits.UintSize == 32 {
		return value >= minInt32 && value <= maxInt32
	}
	return true
}

func numericFloat64(value pgtype.Numeric) (float64, bool, error) {
	if !value.Valid {
		return 0, false, nil
	}

	v, err := value.Float64Value()
	if err != nil {
		return 0, false, err
	}
	if !v.Valid || math.IsNaN(v.Float64) || math.IsInf(v.Float64, 0) {
		return 0, false, fmt.Errorf("pgconv: numeric value is not a finite float64")
	}
	return v.Float64, true, nil
}

func numericInt64(value pgtype.Numeric) (int64, bool, error) {
	if !value.Valid {
		return 0, false, nil
	}

	v, err := value.Int64Value()
	if err != nil {
		return 0, false, err
	}
	if !v.Valid {
		return 0, false, fmt.Errorf("pgconv: numeric value is not a valid int64")
	}
	return v.Int64, true, nil
}

// TextBuilder unwraps pgtype.Text values with optional NULL fallback behavior.
type TextBuilder struct {
	value nullableValue[string]
}

// Fallback sets the value returned when the source text is NULL.
func (b TextBuilder) Fallback(value string) TextBuilder {
	b.value = b.value.withFallback(value)
	return b
}

// Value returns the text value or the configured fallback.
func (b TextBuilder) Value() string {
	value, _ := b.value.resolve("")
	return value
}

// Ptr returns a pointer to the text value or nil when the source is NULL and no fallback is configured.
func (b TextBuilder) Ptr() *string {
	value, ok := b.value.resolve("")
	if !ok {
		return nil
	}
	return ptrOf(value)
}

// Fill writes the text value into ptr when the source is valid or a fallback is configured.
func (b TextBuilder) Fill(ptr *string) {
	if value, ok := b.value.resolve(""); ok {
		*ptr = value
	}
}

// BoolBuilder unwraps pgtype.Bool values with optional NULL fallback behavior.
type BoolBuilder struct {
	value nullableValue[bool]
}

// Fallback sets the value returned when the source bool is NULL.
func (b BoolBuilder) Fallback(value bool) BoolBuilder {
	b.value = b.value.withFallback(value)
	return b
}

// Value returns the bool value or the configured fallback.
func (b BoolBuilder) Value() bool {
	value, _ := b.value.resolve(false)
	return value
}

// Ptr returns a pointer to the bool value or nil when the source is NULL and no fallback is configured.
func (b BoolBuilder) Ptr() *bool {
	value, ok := b.value.resolve(false)
	if !ok {
		return nil
	}
	return ptrOf(value)
}

// Fill writes the bool value into ptr when the source is valid or a fallback is configured.
func (b BoolBuilder) Fill(ptr *bool) {
	if value, ok := b.value.resolve(false); ok {
		*ptr = value
	}
}

// Int2Builder unwraps pgtype.Int2 values with optional NULL fallback behavior.
type Int2Builder struct {
	value nullableValue[int16]
}

// Fallback sets the value returned when the source int2 is NULL.
func (b Int2Builder) Fallback(value int16) Int2Builder {
	b.value = b.value.withFallback(value)
	return b
}

// Value returns the int16 value or the configured fallback.
func (b Int2Builder) Value() int16 {
	value, _ := b.value.resolve(0)
	return value
}

// Ptr returns a pointer to the int16 value or nil when the source is NULL and no fallback is configured.
func (b Int2Builder) Ptr() *int16 {
	value, ok := b.value.resolve(0)
	if !ok {
		return nil
	}
	return ptrOf(value)
}

// Fill writes the int16 value into ptr when the source is valid or a fallback is configured.
func (b Int2Builder) Fill(ptr *int16) {
	if value, ok := b.value.resolve(0); ok {
		*ptr = value
	}
}

// Int returns an int-oriented builder for the wrapped int2 value.
func (b Int2Builder) Int() IntBuilder {
	return IntBuilder{
		resolve: func() (int, bool, error) {
			value, ok := b.value.resolve(0)
			if !ok {
				return 0, false, nil
			}
			return int(value), true, nil
		},
	}
}

// Int4Builder unwraps pgtype.Int4 values with optional NULL fallback behavior.
type Int4Builder struct {
	value nullableValue[int32]
}

// Fallback sets the value returned when the source int4 is NULL.
func (b Int4Builder) Fallback(value int32) Int4Builder {
	b.value = b.value.withFallback(value)
	return b
}

// Value returns the int32 value or the configured fallback.
func (b Int4Builder) Value() int32 {
	value, _ := b.value.resolve(0)
	return value
}

// Ptr returns a pointer to the int32 value or nil when the source is NULL and no fallback is configured.
func (b Int4Builder) Ptr() *int32 {
	value, ok := b.value.resolve(0)
	if !ok {
		return nil
	}
	return ptrOf(value)
}

// Fill writes the int32 value into ptr when the source is valid or a fallback is configured.
func (b Int4Builder) Fill(ptr *int32) {
	if value, ok := b.value.resolve(0); ok {
		*ptr = value
	}
}

// Int returns an int-oriented builder for the wrapped int4 value.
func (b Int4Builder) Int() IntBuilder {
	return IntBuilder{
		resolve: func() (int, bool, error) {
			value, ok := b.value.resolve(0)
			if !ok {
				return 0, false, nil
			}
			return int(value), true, nil
		},
	}
}

// Int8Builder unwraps pgtype.Int8 values with optional NULL fallback behavior.
type Int8Builder struct {
	value nullableValue[int64]
}

// Fallback sets the value returned when the source int8 is NULL.
func (b Int8Builder) Fallback(value int64) Int8Builder {
	b.value = b.value.withFallback(value)
	return b
}

// Value returns the int64 value or the configured fallback.
func (b Int8Builder) Value() int64 {
	value, _ := b.value.resolve(0)
	return value
}

// Ptr returns a pointer to the int64 value or nil when the source is NULL and no fallback is configured.
func (b Int8Builder) Ptr() *int64 {
	value, ok := b.value.resolve(0)
	if !ok {
		return nil
	}
	return ptrOf(value)
}

// Fill writes the int64 value into ptr when the source is valid or a fallback is configured.
func (b Int8Builder) Fill(ptr *int64) {
	if value, ok := b.value.resolve(0); ok {
		*ptr = value
	}
}

// Int returns an int-oriented builder for the wrapped int8 value.
func (b Int8Builder) Int() IntBuilder {
	return IntBuilder{
		resolve: func() (int, bool, error) {
			value, ok := b.value.resolve(0)
			if !ok {
				return 0, false, nil
			}
			if !fitsInt64InInt(value) {
				return 0, false, fmt.Errorf("pgconv: %d overflows int", value)
			}
			return int(value), true, nil
		},
	}
}

// IntBuilder unwraps integer conversions that can fail due to width checks.
type IntBuilder struct {
	fallback    int
	hasFallback bool
	resolve     func() (int, bool, error)
}

// Fallback sets the value returned when the source integer is NULL.
func (b IntBuilder) Fallback(value int) IntBuilder {
	b.fallback = value
	b.hasFallback = true
	return b
}

func (b IntBuilder) resolved() (int, bool, error) {
	value, ok, err := b.resolve()
	if err != nil {
		return 0, false, err
	}
	if ok {
		return value, true, nil
	}
	if b.hasFallback {
		return b.fallback, true, nil
	}
	return 0, false, nil
}

// Value returns the int value or the configured fallback.
func (b IntBuilder) Value() (int, error) {
	value, _, err := b.resolved()
	return value, err
}

// Ptr returns a pointer to the int value or nil when the source is NULL and no fallback is configured.
func (b IntBuilder) Ptr() (*int, error) {
	value, ok, err := b.resolved()
	if err != nil || !ok {
		return nil, err
	}
	return ptrOf(value), nil
}

// Fill writes the int value into ptr when the source is valid or a fallback is configured.
func (b IntBuilder) Fill(ptr *int) error {
	value, ok, err := b.resolved()
	if err != nil {
		return err
	}
	if ok {
		*ptr = value
	}
	return nil
}

// Float8Builder unwraps pgtype.Float8 values with optional NULL fallback behavior.
type Float8Builder struct {
	value nullableValue[float64]
}

// Fallback sets the value returned when the source float8 is NULL.
func (b Float8Builder) Fallback(value float64) Float8Builder {
	b.value = b.value.withFallback(value)
	return b
}

// Value returns the float64 value or the configured fallback.
func (b Float8Builder) Value() float64 {
	value, _ := b.value.resolve(0)
	return value
}

// Ptr returns a pointer to the float64 value or nil when the source is NULL and no fallback is configured.
func (b Float8Builder) Ptr() *float64 {
	value, ok := b.value.resolve(0)
	if !ok {
		return nil
	}
	return ptrOf(value)
}

// Fill writes the float64 value into ptr when the source is valid or a fallback is configured.
func (b Float8Builder) Fill(ptr *float64) {
	if value, ok := b.value.resolve(0); ok {
		*ptr = value
	}
}

// DateBuilder unwraps pgtype.Date values and treats infinities as absent values.
type DateBuilder struct {
	value nullableValue[time.Time]
}

// Fallback sets the value returned when the source date is NULL or non-finite.
func (b DateBuilder) Fallback(value time.Time) DateBuilder {
	b.value = b.value.withFallback(value)
	return b
}

// Value returns the time value or the configured fallback.
func (b DateBuilder) Value() time.Time {
	value, _ := b.value.resolve(time.Time{})
	return value
}

// Ptr returns a pointer to the time value or nil when the source is NULL/non-finite and no fallback is configured.
func (b DateBuilder) Ptr() *time.Time {
	value, ok := b.value.resolve(time.Time{})
	if !ok {
		return nil
	}
	return ptrOf(value)
}

// Fill writes the time value into ptr when the source is finite or a fallback is configured.
func (b DateBuilder) Fill(ptr *time.Time) {
	if value, ok := b.value.resolve(time.Time{}); ok {
		*ptr = value
	}
}

// TimestampBuilder unwraps pgtype.Timestamp values and treats infinities as absent values.
type TimestampBuilder struct {
	value nullableValue[time.Time]
}

// Fallback sets the value returned when the source timestamp is NULL or non-finite.
func (b TimestampBuilder) Fallback(value time.Time) TimestampBuilder {
	b.value = b.value.withFallback(value)
	return b
}

// Value returns the time value or the configured fallback.
func (b TimestampBuilder) Value() time.Time {
	value, _ := b.value.resolve(time.Time{})
	return value
}

// Ptr returns a pointer to the time value or nil when the source is NULL/non-finite and no fallback is configured.
func (b TimestampBuilder) Ptr() *time.Time {
	value, ok := b.value.resolve(time.Time{})
	if !ok {
		return nil
	}
	return ptrOf(value)
}

// Fill writes the time value into ptr when the source is finite or a fallback is configured.
func (b TimestampBuilder) Fill(ptr *time.Time) {
	if value, ok := b.value.resolve(time.Time{}); ok {
		*ptr = value
	}
}

// TimestamptzBuilder unwraps pgtype.Timestamptz values and treats infinities as absent values.
type TimestamptzBuilder struct {
	value nullableValue[time.Time]
}

// Fallback sets the value returned when the source timestamptz is NULL or non-finite.
func (b TimestamptzBuilder) Fallback(value time.Time) TimestamptzBuilder {
	b.value = b.value.withFallback(value)
	return b
}

// Value returns the time value or the configured fallback.
func (b TimestamptzBuilder) Value() time.Time {
	value, _ := b.value.resolve(time.Time{})
	return value
}

// Ptr returns a pointer to the time value or nil when the source is NULL/non-finite and no fallback is configured.
func (b TimestamptzBuilder) Ptr() *time.Time {
	value, ok := b.value.resolve(time.Time{})
	if !ok {
		return nil
	}
	return ptrOf(value)
}

// Fill writes the time value into ptr when the source is finite or a fallback is configured.
func (b TimestamptzBuilder) Fill(ptr *time.Time) {
	if value, ok := b.value.resolve(time.Time{}); ok {
		*ptr = value
	}
}

// UUIDBuilder unwraps pgtype.UUID values with optional NULL fallback behavior.
type UUIDBuilder struct {
	value nullableValue[uuid.UUID]
}

// Fallback sets the value returned when the source UUID is NULL.
func (b UUIDBuilder) Fallback(value uuid.UUID) UUIDBuilder {
	b.value = b.value.withFallback(value)
	return b
}

// Value returns the UUID value or the configured fallback.
func (b UUIDBuilder) Value() uuid.UUID {
	value, _ := b.value.resolve(uuid.Nil)
	return value
}

// String returns the UUID string value or an empty string when the source is NULL and no fallback is configured.
func (b UUIDBuilder) String() string {
	value, ok := b.value.resolve(uuid.Nil)
	if !ok {
		return ""
	}
	return value.String()
}

// Ptr returns a pointer to the UUID value or nil when the source is NULL and no fallback is configured.
func (b UUIDBuilder) Ptr() *uuid.UUID {
	value, ok := b.value.resolve(uuid.Nil)
	if !ok {
		return nil
	}
	return ptrOf(value)
}

// Fill writes the UUID value into ptr when the source is valid or a fallback is configured.
func (b UUIDBuilder) Fill(ptr *uuid.UUID) {
	if value, ok := b.value.resolve(uuid.Nil); ok {
		*ptr = value
	}
}

// NumericBuilder unwraps pgtype.Numeric values into typed numeric builders.
type NumericBuilder struct {
	value pgtype.Numeric
}

// Float64 returns a float64-oriented builder for the wrapped numeric value.
func (b NumericBuilder) Float64() NumericFloat64Builder {
	return NumericFloat64Builder{value: b.value}
}

// Int64 returns an int64-oriented builder for the wrapped numeric value.
func (b NumericBuilder) Int64() NumericInt64Builder {
	return NumericInt64Builder{value: b.value}
}

// Int returns an int-oriented builder for the wrapped numeric value.
func (b NumericBuilder) Int() IntBuilder {
	return IntBuilder{
		resolve: func() (int, bool, error) {
			value, ok, err := numericInt64(b.value)
			if err != nil || !ok {
				return 0, ok, err
			}
			if !fitsInt64InInt(value) {
				return 0, false, fmt.Errorf("pgconv: %d overflows int", value)
			}
			return int(value), true, nil
		},
	}
}

// NumericFloat64Builder unwraps pgtype.Numeric values into float64 values.
type NumericFloat64Builder struct {
	value       pgtype.Numeric
	fallback    float64
	hasFallback bool
}

// Fallback sets the value returned when the source numeric is NULL.
func (b NumericFloat64Builder) Fallback(value float64) NumericFloat64Builder {
	b.fallback = value
	b.hasFallback = true
	return b
}

func (b NumericFloat64Builder) resolved() (float64, bool, error) {
	value, ok, err := numericFloat64(b.value)
	if err != nil {
		return 0, false, err
	}
	if ok {
		return value, true, nil
	}
	if b.hasFallback {
		return b.fallback, true, nil
	}
	return 0, false, nil
}

// Value returns the float64 value or the configured fallback.
func (b NumericFloat64Builder) Value() (float64, error) {
	value, _, err := b.resolved()
	return value, err
}

// Ptr returns a pointer to the float64 value or nil when the source is NULL and no fallback is configured.
func (b NumericFloat64Builder) Ptr() (*float64, error) {
	value, ok, err := b.resolved()
	if err != nil || !ok {
		return nil, err
	}
	return ptrOf(value), nil
}

// Fill writes the float64 value into ptr when the source is valid or a fallback is configured.
func (b NumericFloat64Builder) Fill(ptr *float64) error {
	value, ok, err := b.resolved()
	if err != nil {
		return err
	}
	if ok {
		*ptr = value
	}
	return nil
}

// NumericInt64Builder unwraps pgtype.Numeric values into int64 values.
type NumericInt64Builder struct {
	value       pgtype.Numeric
	fallback    int64
	hasFallback bool
}

// Fallback sets the value returned when the source numeric is NULL.
func (b NumericInt64Builder) Fallback(value int64) NumericInt64Builder {
	b.fallback = value
	b.hasFallback = true
	return b
}

func (b NumericInt64Builder) resolved() (int64, bool, error) {
	value, ok, err := numericInt64(b.value)
	if err != nil {
		return 0, false, err
	}
	if ok {
		return value, true, nil
	}
	if b.hasFallback {
		return b.fallback, true, nil
	}
	return 0, false, nil
}

// Value returns the int64 value or the configured fallback.
func (b NumericInt64Builder) Value() (int64, error) {
	value, _, err := b.resolved()
	return value, err
}

// Ptr returns a pointer to the int64 value or nil when the source is NULL and no fallback is configured.
func (b NumericInt64Builder) Ptr() (*int64, error) {
	value, ok, err := b.resolved()
	if err != nil || !ok {
		return nil, err
	}
	return ptrOf(value), nil
}

// Fill writes the int64 value into ptr when the source is valid or a fallback is configured.
func (b NumericInt64Builder) Fill(ptr *int64) error {
	value, ok, err := b.resolved()
	if err != nil {
		return err
	}
	if ok {
		*ptr = value
	}
	return nil
}

// ValidText returns a non-NULL pgtype.Text, including empty strings.
func ValidText(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: true}
}

// NText returns a nullable pgtype.Text that treats empty strings as NULL.
func NText(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: value != ""}
}

// TextFromPtr builds pgtype.Text from a string pointer.
func TextFromPtr(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return ValidText(*value)
}

// Text wraps a pgtype.Text for builder-style unwrapping.
func Text(value pgtype.Text) TextBuilder {
	return TextBuilder{value: newNullableValue(value.Valid, value.String)}
}

// NBool returns a non-NULL pgtype.Bool.
func NBool(value bool) pgtype.Bool {
	return pgtype.Bool{Bool: value, Valid: true}
}

// BoolFromPtr builds pgtype.Bool from a bool pointer.
func BoolFromPtr(value *bool) pgtype.Bool {
	if value == nil {
		return pgtype.Bool{}
	}
	return NBool(*value)
}

// Bool wraps a pgtype.Bool for builder-style unwrapping.
func Bool(value pgtype.Bool) BoolBuilder {
	return BoolBuilder{value: newNullableValue(value.Valid, value.Bool)}
}

// NInt2 returns a non-NULL pgtype.Int2.
func NInt2(value int16) pgtype.Int2 {
	return pgtype.Int2{Int16: value, Valid: true}
}

// NInt2FromInt8 returns a non-NULL pgtype.Int2 built from int8.
func NInt2FromInt8(value int8) pgtype.Int2 {
	return NInt2(int16(value))
}

// Int2FromPtr builds pgtype.Int2 from an int16 pointer.
func Int2FromPtr(value *int16) pgtype.Int2 {
	if value == nil {
		return pgtype.Int2{}
	}
	return NInt2(*value)
}

// Int2FromInt8Ptr builds pgtype.Int2 from an int8 pointer.
func Int2FromInt8Ptr(value *int8) pgtype.Int2 {
	if value == nil {
		return pgtype.Int2{}
	}
	return NInt2FromInt8(*value)
}

// Int2 wraps a pgtype.Int2 for builder-style unwrapping.
func Int2(value pgtype.Int2) Int2Builder {
	return Int2Builder{value: newNullableValue(value.Valid, value.Int16)}
}

// NInt4 returns a non-NULL pgtype.Int4.
func NInt4(value int32) pgtype.Int4 {
	return pgtype.Int4{Int32: value, Valid: true}
}

// NInt4FromInt16 returns a non-NULL pgtype.Int4 built from int16.
func NInt4FromInt16(value int16) pgtype.Int4 {
	return NInt4(int32(value))
}

// NInt4FromInt8 returns a non-NULL pgtype.Int4 built from int8.
func NInt4FromInt8(value int8) pgtype.Int4 {
	return NInt4(int32(value))
}

// Int4FromPtr builds pgtype.Int4 from an int32 pointer.
func Int4FromPtr(value *int32) pgtype.Int4 {
	if value == nil {
		return pgtype.Int4{}
	}
	return NInt4(*value)
}

// Int4FromInt16Ptr builds pgtype.Int4 from an int16 pointer.
func Int4FromInt16Ptr(value *int16) pgtype.Int4 {
	if value == nil {
		return pgtype.Int4{}
	}
	return NInt4FromInt16(*value)
}

// Int4FromInt8Ptr builds pgtype.Int4 from an int8 pointer.
func Int4FromInt8Ptr(value *int8) pgtype.Int4 {
	if value == nil {
		return pgtype.Int4{}
	}
	return NInt4FromInt8(*value)
}

// Int4 wraps a pgtype.Int4 for builder-style unwrapping.
func Int4(value pgtype.Int4) Int4Builder {
	return Int4Builder{value: newNullableValue(value.Valid, value.Int32)}
}

// NInt8 returns a non-NULL pgtype.Int8.
func NInt8(value int64) pgtype.Int8 {
	return pgtype.Int8{Int64: value, Valid: true}
}

// NInt8FromInt returns a non-NULL pgtype.Int8 built from int.
func NInt8FromInt(value int) pgtype.Int8 {
	return NInt8(int64(value))
}

// NInt8FromInt32 returns a non-NULL pgtype.Int8 built from int32.
func NInt8FromInt32(value int32) pgtype.Int8 {
	return NInt8(int64(value))
}

// Int8FromPtr builds pgtype.Int8 from an int64 pointer.
func Int8FromPtr(value *int64) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return NInt8(*value)
}

// Int8FromIntPtr builds pgtype.Int8 from an int pointer.
func Int8FromIntPtr(value *int) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return NInt8FromInt(*value)
}

// Int8FromInt32Ptr builds pgtype.Int8 from an int32 pointer.
func Int8FromInt32Ptr(value *int32) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return NInt8FromInt32(*value)
}

// Int8 wraps a pgtype.Int8 for builder-style unwrapping.
func Int8(value pgtype.Int8) Int8Builder {
	return Int8Builder{value: newNullableValue(value.Valid, value.Int64)}
}

// NFloat8 returns a non-NULL pgtype.Float8.
func NFloat8(value float64) pgtype.Float8 {
	return pgtype.Float8{Float64: value, Valid: true}
}

// Float8FromPtr builds pgtype.Float8 from a float64 pointer.
func Float8FromPtr(value *float64) pgtype.Float8 {
	if value == nil {
		return pgtype.Float8{}
	}
	return NFloat8(*value)
}

// Float8 wraps a pgtype.Float8 for builder-style unwrapping.
func Float8(value pgtype.Float8) Float8Builder {
	return Float8Builder{value: newNullableValue(value.Valid, value.Float64)}
}

// NDate returns a nullable pgtype.Date that treats zero time as NULL.
func NDate(value time.Time) pgtype.Date {
	return pgtype.Date{Time: value, Valid: !value.IsZero()}
}

// DateFromPtr builds pgtype.Date from a time pointer.
func DateFromPtr(value *time.Time) pgtype.Date {
	if value == nil {
		return pgtype.Date{}
	}
	return NDate(*value)
}

// Date wraps a pgtype.Date for builder-style unwrapping.
func Date(value pgtype.Date) DateBuilder {
	return DateBuilder{value: newNullableValue(finiteDate(value), value.Time)}
}

// NTimestamp returns a nullable pgtype.Timestamp that treats zero time as NULL.
func NTimestamp(value time.Time) pgtype.Timestamp {
	return pgtype.Timestamp{Time: value, Valid: !value.IsZero(), InfinityModifier: pgtype.Finite}
}

// TimestampFromPtr builds pgtype.Timestamp from a time pointer.
func TimestampFromPtr(value *time.Time) pgtype.Timestamp {
	if value == nil {
		return pgtype.Timestamp{}
	}
	return NTimestamp(*value)
}

// Timestamp wraps a pgtype.Timestamp for builder-style unwrapping.
func Timestamp(value pgtype.Timestamp) TimestampBuilder {
	return TimestampBuilder{value: newNullableValue(finiteTimestamp(value), value.Time)}
}

// NTimestamptz returns a nullable pgtype.Timestamptz that treats zero time as NULL.
func NTimestamptz(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: !value.IsZero(), InfinityModifier: pgtype.Finite}
}

// TimestamptzFromPtr builds pgtype.Timestamptz from a time pointer.
func TimestamptzFromPtr(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return NTimestamptz(*value)
}

// Timestamptz wraps a pgtype.Timestamptz for builder-style unwrapping.
func Timestamptz(value pgtype.Timestamptz) TimestamptzBuilder {
	return TimestamptzBuilder{value: newNullableValue(finiteTimestamptz(value), value.Time)}
}

// NUUID returns a nullable pgtype.UUID that treats uuid.Nil as NULL.
func NUUID(value uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte(value), Valid: value != uuid.Nil}
}

// UUIDFromPtr builds pgtype.UUID from a UUID pointer.
func UUIDFromPtr(value *uuid.UUID) pgtype.UUID {
	if value == nil {
		return pgtype.UUID{}
	}
	return NUUID(*value)
}

// UUID wraps a pgtype.UUID for builder-style unwrapping.
func UUID(value pgtype.UUID) UUIDBuilder {
	return UUIDBuilder{value: newNullableValue(value.Valid, uuid.UUID(value.Bytes))}
}

// Numeric wraps a pgtype.Numeric for builder-style unwrapping.
func Numeric(value pgtype.Numeric) NumericBuilder {
	return NumericBuilder{value: value}
}
