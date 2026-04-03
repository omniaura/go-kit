package sqldecode

import (
	"database/sql"
	"fmt"
	"math/bits"
	"time"
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

func fitsInt64InInt(value int64) bool {
	if bits.UintSize == 32 {
		return value >= minInt32 && value <= maxInt32
	}
	return true
}

type stringBuilder struct {
	value nullableValue[string]
}

func (b stringBuilder) Fallback(value string) stringBuilder {
	b.value = b.value.withFallback(value)
	return b
}

func (b stringBuilder) Value() string {
	value, _ := b.value.resolve("")
	return value
}

func (b stringBuilder) Ptr() *string {
	value, ok := b.value.resolve("")
	if !ok {
		return nil
	}
	return ptrOf(value)
}

func (b stringBuilder) Fill(ptr *string) {
	if value, ok := b.value.resolve(""); ok {
		*ptr = value
	}
}

type boolBuilder struct {
	value nullableValue[bool]
}

func (b boolBuilder) Fallback(value bool) boolBuilder {
	b.value = b.value.withFallback(value)
	return b
}

func (b boolBuilder) Value() bool {
	value, _ := b.value.resolve(false)
	return value
}

func (b boolBuilder) Ptr() *bool {
	value, ok := b.value.resolve(false)
	if !ok {
		return nil
	}
	return ptrOf(value)
}

func (b boolBuilder) Fill(ptr *bool) {
	if value, ok := b.value.resolve(false); ok {
		*ptr = value
	}
}

type int16Builder struct {
	value nullableValue[int16]
}

func (b int16Builder) Fallback(value int16) int16Builder {
	b.value = b.value.withFallback(value)
	return b
}

func (b int16Builder) Value() int16 {
	value, _ := b.value.resolve(0)
	return value
}

func (b int16Builder) Ptr() *int16 {
	value, ok := b.value.resolve(0)
	if !ok {
		return nil
	}
	return ptrOf(value)
}

func (b int16Builder) Fill(ptr *int16) {
	if value, ok := b.value.resolve(0); ok {
		*ptr = value
	}
}

func (b int16Builder) Int() safeIntBuilder {
	return safeIntBuilder{
		resolve: func() (int, bool) {
			value, ok := b.value.resolve(0)
			if !ok {
				return 0, false
			}
			return int(value), true
		},
	}
}

type int32Builder struct {
	value nullableValue[int32]
}

func (b int32Builder) Fallback(value int32) int32Builder {
	b.value = b.value.withFallback(value)
	return b
}

func (b int32Builder) Value() int32 {
	value, _ := b.value.resolve(0)
	return value
}

func (b int32Builder) Ptr() *int32 {
	value, ok := b.value.resolve(0)
	if !ok {
		return nil
	}
	return ptrOf(value)
}

func (b int32Builder) Fill(ptr *int32) {
	if value, ok := b.value.resolve(0); ok {
		*ptr = value
	}
}

func (b int32Builder) Int() safeIntBuilder {
	return safeIntBuilder{
		resolve: func() (int, bool) {
			value, ok := b.value.resolve(0)
			if !ok {
				return 0, false
			}
			return int(value), true
		},
	}
}

type int64Builder struct {
	value nullableValue[int64]
}

func (b int64Builder) Fallback(value int64) int64Builder {
	b.value = b.value.withFallback(value)
	return b
}

func (b int64Builder) Value() int64 {
	value, _ := b.value.resolve(0)
	return value
}

func (b int64Builder) Ptr() *int64 {
	value, ok := b.value.resolve(0)
	if !ok {
		return nil
	}
	return ptrOf(value)
}

func (b int64Builder) Fill(ptr *int64) {
	if value, ok := b.value.resolve(0); ok {
		*ptr = value
	}
}

func (b int64Builder) Int() safeIntBuilder {
	return safeIntBuilder{
		resolve: func() (int, bool) {
			value, ok := b.value.resolve(0)
			if !ok {
				return 0, false
			}
			return int(value), true
		},
	}
}

func (b int64Builder) TryInt() intBuilder {
	return intBuilder{
		resolve: func() (int, bool, error) {
			value, ok := b.value.resolve(0)
			if !ok {
				return 0, false, nil
			}
			if !fitsInt64InInt(value) {
				return 0, false, fmt.Errorf("sqldecode: %d overflows int", value)
			}
			return int(value), true, nil
		},
	}
}

type safeIntBuilder struct {
	fallback    int
	hasFallback bool
	resolve     func() (int, bool)
}

func (b safeIntBuilder) Fallback(value int) safeIntBuilder {
	b.fallback = value
	b.hasFallback = true
	return b
}

func (b safeIntBuilder) resolved() (int, bool) {
	value, ok := b.resolve()
	if ok {
		return value, true
	}
	if b.hasFallback {
		return b.fallback, true
	}
	return 0, false
}

func (b safeIntBuilder) Value() int {
	value, _ := b.resolved()
	return value
}

func (b safeIntBuilder) Ptr() *int {
	value, ok := b.resolved()
	if !ok {
		return nil
	}
	return ptrOf(value)
}

func (b safeIntBuilder) Fill(ptr *int) {
	if value, ok := b.resolved(); ok {
		*ptr = value
	}
}

type intBuilder struct {
	fallback    int
	hasFallback bool
	resolve     func() (int, bool, error)
}

func (b intBuilder) Fallback(value int) intBuilder {
	b.fallback = value
	b.hasFallback = true
	return b
}

func (b intBuilder) resolved() (int, bool, error) {
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

func (b intBuilder) Value() (int, error) {
	value, _, err := b.resolved()
	return value, err
}

func (b intBuilder) Ptr() (*int, error) {
	value, ok, err := b.resolved()
	if err != nil || !ok {
		return nil, err
	}
	return ptrOf(value), nil
}

func (b intBuilder) Fill(ptr *int) error {
	value, ok, err := b.resolved()
	if err != nil {
		return err
	}
	if ok {
		*ptr = value
	}
	return nil
}

type float64Builder struct {
	value nullableValue[float64]
}

func (b float64Builder) Fallback(value float64) float64Builder {
	b.value = b.value.withFallback(value)
	return b
}

func (b float64Builder) Value() float64 {
	value, _ := b.value.resolve(0)
	return value
}

func (b float64Builder) Ptr() *float64 {
	value, ok := b.value.resolve(0)
	if !ok {
		return nil
	}
	return ptrOf(value)
}

func (b float64Builder) Fill(ptr *float64) {
	if value, ok := b.value.resolve(0); ok {
		*ptr = value
	}
}

type timeBuilder struct {
	value nullableValue[time.Time]
}

func (b timeBuilder) Fallback(value time.Time) timeBuilder {
	b.value = b.value.withFallback(value)
	return b
}

func (b timeBuilder) Value() time.Time {
	value, _ := b.value.resolve(time.Time{})
	return value
}

func (b timeBuilder) Ptr() *time.Time {
	value, ok := b.value.resolve(time.Time{})
	if !ok {
		return nil
	}
	return ptrOf(value)
}

func (b timeBuilder) Fill(ptr *time.Time) {
	if value, ok := b.value.resolve(time.Time{}); ok {
		*ptr = value
	}
}

func String(value sql.NullString) stringBuilder {
	return stringBuilder{value: newNullableValue(value.Valid, value.String)}
}

func Bool(value sql.NullBool) boolBuilder {
	return boolBuilder{value: newNullableValue(value.Valid, value.Bool)}
}

func Int16(value sql.NullInt16) int16Builder {
	return int16Builder{value: newNullableValue(value.Valid, value.Int16)}
}

func Int32(value sql.NullInt32) int32Builder {
	return int32Builder{value: newNullableValue(value.Valid, value.Int32)}
}

func Int64(value sql.NullInt64) int64Builder {
	return int64Builder{value: newNullableValue(value.Valid, value.Int64)}
}

func Float64(value sql.NullFloat64) float64Builder {
	return float64Builder{value: newNullableValue(value.Valid, value.Float64)}
}

func Time(value sql.NullTime) timeBuilder {
	return timeBuilder{value: newNullableValue(value.Valid, value.Time)}
}
