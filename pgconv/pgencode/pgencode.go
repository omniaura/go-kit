package pgencode

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type inputValue[T any] struct {
	value   T
	present bool
}

func newValueInput[T any](value T) inputValue[T] {
	return inputValue[T]{value: value, present: true}
}

func newPtrInput[T any](value *T) inputValue[T] {
	if value == nil {
		return inputValue[T]{}
	}
	return inputValue[T]{value: *value, present: true}
}

func int2Value(value int64, present bool) (pgtype.Int2, error) {
	if !present {
		return pgtype.Int2{}, nil
	}
	if value < math.MinInt16 || value > math.MaxInt16 {
		return pgtype.Int2{}, fmt.Errorf("pgencode: %d overflows pgtype.Int2", value)
	}
	return pgtype.Int2{Int16: int16(value), Valid: true}, nil
}

func int4Value(value int64, present bool) (pgtype.Int4, error) {
	if !present {
		return pgtype.Int4{}, nil
	}
	if value < math.MinInt32 || value > math.MaxInt32 {
		return pgtype.Int4{}, fmt.Errorf("pgencode: %d overflows pgtype.Int4", value)
	}
	return pgtype.Int4{Int32: int32(value), Valid: true}, nil
}

func int8Value(value int64, present bool) pgtype.Int8 {
	if !present {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: value, Valid: true}
}

func int2TruncatedValue(value int64, present bool) pgtype.Int2 {
	if !present {
		return pgtype.Int2{}
	}
	return pgtype.Int2{Int16: int16(value), Valid: true}
}

func int4TruncatedValue(value int64, present bool) pgtype.Int4 {
	if !present {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: int32(value), Valid: true}
}

type stringBuilder struct {
	input       inputValue[string]
	emptyIsNull bool
	trimSpace   bool
}

func (b stringBuilder) EmptyIsNull() stringBuilder {
	b.emptyIsNull = true
	return b
}

// TrimSpace trims leading and trailing whitespace (via strings.TrimSpace) from
// the input before the value and validity are computed. It composes with
// EmptyIsNull() in either order, so pgencode.String(v).TrimSpace().EmptyIsNull().Text()
// yields a NULL pgtype.Text when v is empty or all-whitespace.
func (b stringBuilder) TrimSpace() stringBuilder {
	b.trimSpace = true
	return b
}

func (b stringBuilder) resolved() string {
	if b.trimSpace {
		return strings.TrimSpace(b.input.value)
	}
	return b.input.value
}

func (b stringBuilder) Text() pgtype.Text {
	if !b.input.present {
		return pgtype.Text{}
	}
	value := b.resolved()
	if b.emptyIsNull && value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
}

type boolBuilder struct {
	input inputValue[bool]
}

func (b boolBuilder) Bool() pgtype.Bool {
	if !b.input.present {
		return pgtype.Bool{}
	}
	return pgtype.Bool{Bool: b.input.value, Valid: true}
}

// intNullMode records the optional NULL-selection modifier requested on an
// integer builder. Zero value (intNullNone) means "only a nil pointer is NULL".
type intNullMode uint8

const (
	intNullNone        intNullMode = iota // no modifier
	intNullZero                           // ZeroIsNull(): NULL when value == 0
	intNullNonPositive                    // NonPositiveIsNull(): NULL when value <= 0
)

// intPresent reports whether an integer builder should produce a valid
// (non-NULL) value. It is false when the input is absent (e.g. a nil pointer)
// or when the requested nullMode selects the value as NULL: ZeroIsNull when the
// value is zero, NonPositiveIsNull when the value is <= 0.
func intPresent(value int64, present bool, nullMode intNullMode) bool {
	if !present {
		return false
	}
	switch nullMode {
	case intNullZero:
		return value != 0
	case intNullNonPositive:
		return value > 0
	default:
		return true
	}
}

type int8Builder struct {
	input    inputValue[int8]
	nullMode intNullMode
}

// ZeroIsNull makes the resulting pgtype value NULL when the input integer is 0.
func (b int8Builder) ZeroIsNull() int8Builder {
	b.nullMode = intNullZero
	return b
}

// NonPositiveIsNull makes the resulting pgtype value NULL when the input integer
// is <= 0 (zero or negative).
func (b int8Builder) NonPositiveIsNull() int8Builder {
	b.nullMode = intNullNonPositive
	return b
}

func (b int8Builder) present() bool {
	return intPresent(int64(b.input.value), b.input.present, b.nullMode)
}

func (b int8Builder) Int2() pgtype.Int2 {
	return int2TruncatedValue(int64(b.input.value), b.present())
}

func (b int8Builder) Int4() pgtype.Int4 {
	return int4TruncatedValue(int64(b.input.value), b.present())
}

func (b int8Builder) Int8() pgtype.Int8 {
	return int8Value(int64(b.input.value), b.present())
}

type int16Builder struct {
	input    inputValue[int16]
	nullMode intNullMode
}

// ZeroIsNull makes the resulting pgtype value NULL when the input integer is 0.
func (b int16Builder) ZeroIsNull() int16Builder {
	b.nullMode = intNullZero
	return b
}

// NonPositiveIsNull makes the resulting pgtype value NULL when the input integer
// is <= 0 (zero or negative).
func (b int16Builder) NonPositiveIsNull() int16Builder {
	b.nullMode = intNullNonPositive
	return b
}

func (b int16Builder) present() bool {
	return intPresent(int64(b.input.value), b.input.present, b.nullMode)
}

func (b int16Builder) Int2() pgtype.Int2 {
	return int2TruncatedValue(int64(b.input.value), b.present())
}

func (b int16Builder) Int4() pgtype.Int4 {
	return int4TruncatedValue(int64(b.input.value), b.present())
}

func (b int16Builder) Int8() pgtype.Int8 {
	return int8Value(int64(b.input.value), b.present())
}

type int32Builder struct {
	input    inputValue[int32]
	nullMode intNullMode
}

// ZeroIsNull makes the resulting pgtype value NULL when the input integer is 0.
func (b int32Builder) ZeroIsNull() int32Builder {
	b.nullMode = intNullZero
	return b
}

// NonPositiveIsNull makes the resulting pgtype value NULL when the input integer
// is <= 0 (zero or negative).
func (b int32Builder) NonPositiveIsNull() int32Builder {
	b.nullMode = intNullNonPositive
	return b
}

func (b int32Builder) present() bool {
	return intPresent(int64(b.input.value), b.input.present, b.nullMode)
}

func (b int32Builder) Int2() pgtype.Int2 {
	return int2TruncatedValue(int64(b.input.value), b.present())
}

func (b int32Builder) TryInt2() (pgtype.Int2, error) {
	return int2Value(int64(b.input.value), b.present())
}

func (b int32Builder) Int4() pgtype.Int4 {
	return int4TruncatedValue(int64(b.input.value), b.present())
}

func (b int32Builder) Int8() pgtype.Int8 {
	return int8Value(int64(b.input.value), b.present())
}

type int64Builder struct {
	input    inputValue[int64]
	nullMode intNullMode
}

// ZeroIsNull makes the resulting pgtype value NULL when the input integer is 0.
func (b int64Builder) ZeroIsNull() int64Builder {
	b.nullMode = intNullZero
	return b
}

// NonPositiveIsNull makes the resulting pgtype value NULL when the input integer
// is <= 0 (zero or negative).
func (b int64Builder) NonPositiveIsNull() int64Builder {
	b.nullMode = intNullNonPositive
	return b
}

func (b int64Builder) present() bool {
	return intPresent(b.input.value, b.input.present, b.nullMode)
}

func (b int64Builder) Int2() pgtype.Int2 {
	return int2TruncatedValue(b.input.value, b.present())
}

func (b int64Builder) TryInt2() (pgtype.Int2, error) {
	return int2Value(b.input.value, b.present())
}

func (b int64Builder) Int4() pgtype.Int4 {
	return int4TruncatedValue(b.input.value, b.present())
}

func (b int64Builder) TryInt4() (pgtype.Int4, error) {
	return int4Value(b.input.value, b.present())
}

func (b int64Builder) Int8() pgtype.Int8 {
	return int8Value(b.input.value, b.present())
}

type intBuilder struct {
	input    inputValue[int]
	nullMode intNullMode
}

// ZeroIsNull makes the resulting pgtype value NULL when the input integer is 0.
func (b intBuilder) ZeroIsNull() intBuilder {
	b.nullMode = intNullZero
	return b
}

// NonPositiveIsNull makes the resulting pgtype value NULL when the input integer
// is <= 0 (zero or negative).
func (b intBuilder) NonPositiveIsNull() intBuilder {
	b.nullMode = intNullNonPositive
	return b
}

func (b intBuilder) present() bool {
	return intPresent(int64(b.input.value), b.input.present, b.nullMode)
}

func (b intBuilder) Int2() pgtype.Int2 {
	return int2TruncatedValue(int64(b.input.value), b.present())
}

func (b intBuilder) TryInt2() (pgtype.Int2, error) {
	return int2Value(int64(b.input.value), b.present())
}

func (b intBuilder) Int4() pgtype.Int4 {
	return int4TruncatedValue(int64(b.input.value), b.present())
}

func (b intBuilder) TryInt4() (pgtype.Int4, error) {
	return int4Value(int64(b.input.value), b.present())
}

func (b intBuilder) Int8() pgtype.Int8 {
	return int8Value(int64(b.input.value), b.present())
}

type float64Builder struct {
	input    inputValue[float64]
	nullMode intNullMode
}

// ZeroIsNull makes the resulting pgtype value NULL when the input float is 0.
func (b float64Builder) ZeroIsNull() float64Builder {
	b.nullMode = intNullZero
	return b
}

// NonPositiveIsNull makes the resulting pgtype value NULL when the input float
// is <= 0 (zero or negative).
func (b float64Builder) NonPositiveIsNull() float64Builder {
	b.nullMode = intNullNonPositive
	return b
}

func (b float64Builder) valid() bool {
	if !b.input.present {
		return false
	}
	switch b.nullMode {
	case intNullZero:
		return b.input.value != 0
	case intNullNonPositive:
		return b.input.value > 0
	default:
		return true
	}
}

func (b float64Builder) Float8() pgtype.Float8 {
	if !b.valid() {
		return pgtype.Float8{}
	}
	return pgtype.Float8{Float64: b.input.value, Valid: true}
}

type timeBuilder struct {
	input      inputValue[time.Time]
	zeroIsNull bool
}

func (b timeBuilder) ZeroIsNull() timeBuilder {
	b.zeroIsNull = true
	return b
}

func (b timeBuilder) valid() bool {
	if !b.input.present {
		return false
	}
	if b.zeroIsNull && b.input.value.IsZero() {
		return false
	}
	return true
}

func (b timeBuilder) Date() pgtype.Date {
	if !b.valid() {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: b.input.value, Valid: true, InfinityModifier: pgtype.Finite}
}

func (b timeBuilder) Timestamp() pgtype.Timestamp {
	if !b.valid() {
		return pgtype.Timestamp{}
	}
	return pgtype.Timestamp{Time: b.input.value, Valid: true, InfinityModifier: pgtype.Finite}
}

func (b timeBuilder) Timestamptz() pgtype.Timestamptz {
	if !b.valid() {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: b.input.value, Valid: true, InfinityModifier: pgtype.Finite}
}

type uuidBuilder struct {
	input     inputValue[uuid.UUID]
	nilIsNull bool
}

func (b uuidBuilder) NilIsNull() uuidBuilder {
	b.nilIsNull = true
	return b
}

func (b uuidBuilder) UUID() pgtype.UUID {
	if !b.input.present {
		return pgtype.UUID{}
	}
	if b.nilIsNull && b.input.value == uuid.Nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: [16]byte(b.input.value), Valid: true}
}

func String(value string) stringBuilder {
	return stringBuilder{input: newValueInput(value)}
}

func StringPtr(value *string) stringBuilder {
	return stringBuilder{input: newPtrInput(value)}
}

func Bool(value bool) boolBuilder {
	return boolBuilder{input: newValueInput(value)}
}

func BoolPtr(value *bool) boolBuilder {
	return boolBuilder{input: newPtrInput(value)}
}

func Int8(value int8) int8Builder {
	return int8Builder{input: newValueInput(value)}
}

func Int8Ptr(value *int8) int8Builder {
	return int8Builder{input: newPtrInput(value)}
}

func Int16(value int16) int16Builder {
	return int16Builder{input: newValueInput(value)}
}

func Int16Ptr(value *int16) int16Builder {
	return int16Builder{input: newPtrInput(value)}
}

func Int32(value int32) int32Builder {
	return int32Builder{input: newValueInput(value)}
}

func Int32Ptr(value *int32) int32Builder {
	return int32Builder{input: newPtrInput(value)}
}

func Int64(value int64) int64Builder {
	return int64Builder{input: newValueInput(value)}
}

func Int64Ptr(value *int64) int64Builder {
	return int64Builder{input: newPtrInput(value)}
}

func Int(value int) intBuilder {
	return intBuilder{input: newValueInput(value)}
}

func IntPtr(value *int) intBuilder {
	return intBuilder{input: newPtrInput(value)}
}

func Float64(value float64) float64Builder {
	return float64Builder{input: newValueInput(value)}
}

func Float64Ptr(value *float64) float64Builder {
	return float64Builder{input: newPtrInput(value)}
}

func Time(value time.Time) timeBuilder {
	return timeBuilder{input: newValueInput(value)}
}

func TimePtr(value *time.Time) timeBuilder {
	return timeBuilder{input: newPtrInput(value)}
}

func UUID(value uuid.UUID) uuidBuilder {
	return uuidBuilder{input: newValueInput(value)}
}

func UUIDPtr(value *uuid.UUID) uuidBuilder {
	return uuidBuilder{input: newPtrInput(value)}
}
