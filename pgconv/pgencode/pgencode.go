package pgencode

import (
	"fmt"
	"math"
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

type stringBuilder struct {
	input       inputValue[string]
	emptyIsNull bool
}

func (b stringBuilder) EmptyIsNull() stringBuilder {
	b.emptyIsNull = true
	return b
}

func (b stringBuilder) Text() pgtype.Text {
	if !b.input.present {
		return pgtype.Text{}
	}
	if b.emptyIsNull && b.input.value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: b.input.value, Valid: true}
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

type int8Builder struct {
	input inputValue[int8]
}

func (b int8Builder) Int2() pgtype.Int2 {
	if !b.input.present {
		return pgtype.Int2{}
	}
	return pgtype.Int2{Int16: int16(b.input.value), Valid: true}
}

func (b int8Builder) Int4() pgtype.Int4 {
	if !b.input.present {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: int32(b.input.value), Valid: true}
}

func (b int8Builder) Int8() pgtype.Int8 {
	return int8Value(int64(b.input.value), b.input.present)
}

type int16Builder struct {
	input inputValue[int16]
}

func (b int16Builder) Int2() pgtype.Int2 {
	if !b.input.present {
		return pgtype.Int2{}
	}
	return pgtype.Int2{Int16: b.input.value, Valid: true}
}

func (b int16Builder) Int4() pgtype.Int4 {
	if !b.input.present {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: int32(b.input.value), Valid: true}
}

func (b int16Builder) Int8() pgtype.Int8 {
	return int8Value(int64(b.input.value), b.input.present)
}

type int32Builder struct {
	input inputValue[int32]
}

func (b int32Builder) Int2() (pgtype.Int2, error) {
	return int2Value(int64(b.input.value), b.input.present)
}

func (b int32Builder) Int4() pgtype.Int4 {
	if !b.input.present {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: b.input.value, Valid: true}
}

func (b int32Builder) Int8() pgtype.Int8 {
	return int8Value(int64(b.input.value), b.input.present)
}

type int64Builder struct {
	input inputValue[int64]
}

func (b int64Builder) Int2() (pgtype.Int2, error) {
	return int2Value(b.input.value, b.input.present)
}

func (b int64Builder) Int4() (pgtype.Int4, error) {
	return int4Value(b.input.value, b.input.present)
}

func (b int64Builder) Int8() pgtype.Int8 {
	return int8Value(b.input.value, b.input.present)
}

type intBuilder struct {
	input inputValue[int]
}

func (b intBuilder) Int2() (pgtype.Int2, error) {
	return int2Value(int64(b.input.value), b.input.present)
}

func (b intBuilder) Int4() (pgtype.Int4, error) {
	return int4Value(int64(b.input.value), b.input.present)
}

func (b intBuilder) Int8() pgtype.Int8 {
	return int8Value(int64(b.input.value), b.input.present)
}

type float64Builder struct {
	input inputValue[float64]
}

func (b float64Builder) Float8() pgtype.Float8 {
	if !b.input.present {
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
