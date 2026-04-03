package sqlencode

import (
	"database/sql"
	"fmt"
	"math"
	"time"
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

func int16Value(value int64, present bool) (sql.NullInt16, error) {
	if !present {
		return sql.NullInt16{}, nil
	}
	if value < math.MinInt16 || value > math.MaxInt16 {
		return sql.NullInt16{}, fmt.Errorf("sqlencode: %d overflows sql.NullInt16", value)
	}
	return sql.NullInt16{Int16: int16(value), Valid: true}, nil
}

func int32Value(value int64, present bool) (sql.NullInt32, error) {
	if !present {
		return sql.NullInt32{}, nil
	}
	if value < math.MinInt32 || value > math.MaxInt32 {
		return sql.NullInt32{}, fmt.Errorf("sqlencode: %d overflows sql.NullInt32", value)
	}
	return sql.NullInt32{Int32: int32(value), Valid: true}, nil
}

func int64Value(value int64, present bool) sql.NullInt64 {
	if !present {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: value, Valid: true}
}

func int16TruncatedValue(value int64, present bool) sql.NullInt16 {
	if !present {
		return sql.NullInt16{}
	}
	return sql.NullInt16{Int16: int16(value), Valid: true}
}

func int32TruncatedValue(value int64, present bool) sql.NullInt32 {
	if !present {
		return sql.NullInt32{}
	}
	return sql.NullInt32{Int32: int32(value), Valid: true}
}

type stringBuilder struct {
	input       inputValue[string]
	emptyIsNull bool
}

func (b stringBuilder) EmptyIsNull() stringBuilder {
	b.emptyIsNull = true
	return b
}

func (b stringBuilder) String() sql.NullString {
	if !b.input.present {
		return sql.NullString{}
	}
	if b.emptyIsNull && b.input.value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: b.input.value, Valid: true}
}

type boolBuilder struct {
	input inputValue[bool]
}

func (b boolBuilder) Bool() sql.NullBool {
	if !b.input.present {
		return sql.NullBool{}
	}
	return sql.NullBool{Bool: b.input.value, Valid: true}
}

type int8Builder struct {
	input inputValue[int8]
}

func (b int8Builder) Int16() sql.NullInt16 {
	return int16TruncatedValue(int64(b.input.value), b.input.present)
}

func (b int8Builder) Int32() sql.NullInt32 {
	return int32TruncatedValue(int64(b.input.value), b.input.present)
}

func (b int8Builder) Int64() sql.NullInt64 {
	return int64Value(int64(b.input.value), b.input.present)
}

type int16Builder struct {
	input inputValue[int16]
}

func (b int16Builder) Int16() sql.NullInt16 {
	if !b.input.present {
		return sql.NullInt16{}
	}
	return sql.NullInt16{Int16: b.input.value, Valid: true}
}

func (b int16Builder) Int32() sql.NullInt32 {
	return int32TruncatedValue(int64(b.input.value), b.input.present)
}

func (b int16Builder) Int64() sql.NullInt64 {
	return int64Value(int64(b.input.value), b.input.present)
}

type int32Builder struct {
	input inputValue[int32]
}

func (b int32Builder) Int16() sql.NullInt16 {
	return int16TruncatedValue(int64(b.input.value), b.input.present)
}

func (b int32Builder) TryInt16() (sql.NullInt16, error) {
	return int16Value(int64(b.input.value), b.input.present)
}

func (b int32Builder) Int32() sql.NullInt32 {
	if !b.input.present {
		return sql.NullInt32{}
	}
	return sql.NullInt32{Int32: b.input.value, Valid: true}
}

func (b int32Builder) Int64() sql.NullInt64 {
	return int64Value(int64(b.input.value), b.input.present)
}

type int64Builder struct {
	input inputValue[int64]
}

func (b int64Builder) Int16() sql.NullInt16 {
	return int16TruncatedValue(b.input.value, b.input.present)
}

func (b int64Builder) TryInt16() (sql.NullInt16, error) {
	return int16Value(b.input.value, b.input.present)
}

func (b int64Builder) Int32() sql.NullInt32 {
	return int32TruncatedValue(b.input.value, b.input.present)
}

func (b int64Builder) TryInt32() (sql.NullInt32, error) {
	return int32Value(b.input.value, b.input.present)
}

func (b int64Builder) Int64() sql.NullInt64 {
	return int64Value(b.input.value, b.input.present)
}

type intBuilder struct {
	input inputValue[int]
}

func (b intBuilder) Int16() sql.NullInt16 {
	return int16TruncatedValue(int64(b.input.value), b.input.present)
}

func (b intBuilder) TryInt16() (sql.NullInt16, error) {
	return int16Value(int64(b.input.value), b.input.present)
}

func (b intBuilder) Int32() sql.NullInt32 {
	return int32TruncatedValue(int64(b.input.value), b.input.present)
}

func (b intBuilder) TryInt32() (sql.NullInt32, error) {
	return int32Value(int64(b.input.value), b.input.present)
}

func (b intBuilder) Int64() sql.NullInt64 {
	return int64Value(int64(b.input.value), b.input.present)
}

type float64Builder struct {
	input inputValue[float64]
}

func (b float64Builder) Float64() sql.NullFloat64 {
	if !b.input.present {
		return sql.NullFloat64{}
	}
	return sql.NullFloat64{Float64: b.input.value, Valid: true}
}

type timeBuilder struct {
	input      inputValue[time.Time]
	zeroIsNull bool
}

func (b timeBuilder) ZeroIsNull() timeBuilder {
	b.zeroIsNull = true
	return b
}

func (b timeBuilder) Time() sql.NullTime {
	if !b.input.present {
		return sql.NullTime{}
	}
	if b.zeroIsNull && b.input.value.IsZero() {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: b.input.value, Valid: true}
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
