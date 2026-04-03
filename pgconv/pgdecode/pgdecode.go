package pgdecode

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
		return 0, false, fmt.Errorf("pgdecode: numeric value is not a finite float64")
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
		return 0, false, fmt.Errorf("pgdecode: numeric value is not a valid int64")
	}
	return v.Int64, true, nil
}

type textBuilder struct {
	value nullableValue[string]
}

func (b textBuilder) Fallback(value string) textBuilder {
	b.value = b.value.withFallback(value)
	return b
}

func (b textBuilder) Value() string {
	value, _ := b.value.resolve("")
	return value
}

func (b textBuilder) Ptr() *string {
	value, ok := b.value.resolve("")
	if !ok {
		return nil
	}
	return ptrOf(value)
}

func (b textBuilder) Fill(ptr *string) {
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

type int2Builder struct {
	value nullableValue[int16]
}

func (b int2Builder) Fallback(value int16) int2Builder {
	b.value = b.value.withFallback(value)
	return b
}

func (b int2Builder) Value() int16 {
	value, _ := b.value.resolve(0)
	return value
}

func (b int2Builder) Ptr() *int16 {
	value, ok := b.value.resolve(0)
	if !ok {
		return nil
	}
	return ptrOf(value)
}

func (b int2Builder) Fill(ptr *int16) {
	if value, ok := b.value.resolve(0); ok {
		*ptr = value
	}
}

func (b int2Builder) Int() safeIntBuilder {
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

type int4Builder struct {
	value nullableValue[int32]
}

func (b int4Builder) Fallback(value int32) int4Builder {
	b.value = b.value.withFallback(value)
	return b
}

func (b int4Builder) Value() int32 {
	value, _ := b.value.resolve(0)
	return value
}

func (b int4Builder) Ptr() *int32 {
	value, ok := b.value.resolve(0)
	if !ok {
		return nil
	}
	return ptrOf(value)
}

func (b int4Builder) Fill(ptr *int32) {
	if value, ok := b.value.resolve(0); ok {
		*ptr = value
	}
}

func (b int4Builder) Int() safeIntBuilder {
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

type int8Builder struct {
	value nullableValue[int64]
}

func (b int8Builder) Fallback(value int64) int8Builder {
	b.value = b.value.withFallback(value)
	return b
}

func (b int8Builder) Value() int64 {
	value, _ := b.value.resolve(0)
	return value
}

func (b int8Builder) Ptr() *int64 {
	value, ok := b.value.resolve(0)
	if !ok {
		return nil
	}
	return ptrOf(value)
}

func (b int8Builder) Fill(ptr *int64) {
	if value, ok := b.value.resolve(0); ok {
		*ptr = value
	}
}

func (b int8Builder) Int() safeIntBuilder {
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

func (b int8Builder) TryInt() intBuilder {
	return intBuilder{
		resolve: func() (int, bool, error) {
			value, ok := b.value.resolve(0)
			if !ok {
				return 0, false, nil
			}
			if !fitsInt64InInt(value) {
				return 0, false, fmt.Errorf("pgdecode: %d overflows int", value)
			}
			return int(value), true, nil
		},
	}
}

type intBuilder struct {
	fallback    int
	hasFallback bool
	resolve     func() (int, bool, error)
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

type float8Builder struct {
	value nullableValue[float64]
}

func (b float8Builder) Fallback(value float64) float8Builder {
	b.value = b.value.withFallback(value)
	return b
}

func (b float8Builder) Value() float64 {
	value, _ := b.value.resolve(0)
	return value
}

func (b float8Builder) Ptr() *float64 {
	value, ok := b.value.resolve(0)
	if !ok {
		return nil
	}
	return ptrOf(value)
}

func (b float8Builder) Fill(ptr *float64) {
	if value, ok := b.value.resolve(0); ok {
		*ptr = value
	}
}

type dateBuilder struct {
	value nullableValue[time.Time]
}

func (b dateBuilder) Fallback(value time.Time) dateBuilder {
	b.value = b.value.withFallback(value)
	return b
}

func (b dateBuilder) Value() time.Time {
	value, _ := b.value.resolve(time.Time{})
	return value
}

func (b dateBuilder) Ptr() *time.Time {
	value, ok := b.value.resolve(time.Time{})
	if !ok {
		return nil
	}
	return ptrOf(value)
}

func (b dateBuilder) Fill(ptr *time.Time) {
	if value, ok := b.value.resolve(time.Time{}); ok {
		*ptr = value
	}
}

type timestampBuilder struct {
	value nullableValue[time.Time]
}

func (b timestampBuilder) Fallback(value time.Time) timestampBuilder {
	b.value = b.value.withFallback(value)
	return b
}

func (b timestampBuilder) Value() time.Time {
	value, _ := b.value.resolve(time.Time{})
	return value
}

func (b timestampBuilder) Ptr() *time.Time {
	value, ok := b.value.resolve(time.Time{})
	if !ok {
		return nil
	}
	return ptrOf(value)
}

func (b timestampBuilder) Fill(ptr *time.Time) {
	if value, ok := b.value.resolve(time.Time{}); ok {
		*ptr = value
	}
}

type timestamptzBuilder struct {
	value nullableValue[time.Time]
}

func (b timestamptzBuilder) Fallback(value time.Time) timestamptzBuilder {
	b.value = b.value.withFallback(value)
	return b
}

func (b timestamptzBuilder) Value() time.Time {
	value, _ := b.value.resolve(time.Time{})
	return value
}

func (b timestamptzBuilder) Ptr() *time.Time {
	value, ok := b.value.resolve(time.Time{})
	if !ok {
		return nil
	}
	return ptrOf(value)
}

func (b timestamptzBuilder) Fill(ptr *time.Time) {
	if value, ok := b.value.resolve(time.Time{}); ok {
		*ptr = value
	}
}

type uuidBuilder struct {
	value nullableValue[uuid.UUID]
}

func (b uuidBuilder) Fallback(value uuid.UUID) uuidBuilder {
	b.value = b.value.withFallback(value)
	return b
}

func (b uuidBuilder) Value() uuid.UUID {
	value, _ := b.value.resolve(uuid.Nil)
	return value
}

func (b uuidBuilder) String() string {
	value, ok := b.value.resolve(uuid.Nil)
	if !ok {
		return ""
	}
	return value.String()
}

func (b uuidBuilder) Ptr() *uuid.UUID {
	value, ok := b.value.resolve(uuid.Nil)
	if !ok {
		return nil
	}
	return ptrOf(value)
}

func (b uuidBuilder) Fill(ptr *uuid.UUID) {
	if value, ok := b.value.resolve(uuid.Nil); ok {
		*ptr = value
	}
}

type numericBuilder struct {
	value pgtype.Numeric
}

func (b numericBuilder) Float64() numericFloat64Builder {
	return numericFloat64Builder{value: b.value}
}

func (b numericBuilder) Int64() numericInt64Builder {
	return numericInt64Builder{value: b.value}
}

func (b numericBuilder) Int() intBuilder {
	return intBuilder{
		resolve: func() (int, bool, error) {
			value, ok, err := numericInt64(b.value)
			if err != nil || !ok {
				return 0, ok, err
			}
			if !fitsInt64InInt(value) {
				return 0, false, fmt.Errorf("pgdecode: %d overflows int", value)
			}
			return int(value), true, nil
		},
	}
}

type numericFloat64Builder struct {
	value       pgtype.Numeric
	fallback    float64
	hasFallback bool
}

func (b numericFloat64Builder) Fallback(value float64) numericFloat64Builder {
	b.fallback = value
	b.hasFallback = true
	return b
}

func (b numericFloat64Builder) resolved() (float64, bool, error) {
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

func (b numericFloat64Builder) Value() (float64, error) {
	value, _, err := b.resolved()
	return value, err
}

func (b numericFloat64Builder) Ptr() (*float64, error) {
	value, ok, err := b.resolved()
	if err != nil || !ok {
		return nil, err
	}
	return ptrOf(value), nil
}

func (b numericFloat64Builder) Fill(ptr *float64) error {
	value, ok, err := b.resolved()
	if err != nil {
		return err
	}
	if ok {
		*ptr = value
	}
	return nil
}

type numericInt64Builder struct {
	value       pgtype.Numeric
	fallback    int64
	hasFallback bool
}

func (b numericInt64Builder) Fallback(value int64) numericInt64Builder {
	b.fallback = value
	b.hasFallback = true
	return b
}

func (b numericInt64Builder) resolved() (int64, bool, error) {
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

func (b numericInt64Builder) Value() (int64, error) {
	value, _, err := b.resolved()
	return value, err
}

func (b numericInt64Builder) Ptr() (*int64, error) {
	value, ok, err := b.resolved()
	if err != nil || !ok {
		return nil, err
	}
	return ptrOf(value), nil
}

func (b numericInt64Builder) Fill(ptr *int64) error {
	value, ok, err := b.resolved()
	if err != nil {
		return err
	}
	if ok {
		*ptr = value
	}
	return nil
}

func Text(value pgtype.Text) textBuilder {
	return textBuilder{value: newNullableValue(value.Valid, value.String)}
}

func Bool(value pgtype.Bool) boolBuilder {
	return boolBuilder{value: newNullableValue(value.Valid, value.Bool)}
}

func Int2(value pgtype.Int2) int2Builder {
	return int2Builder{value: newNullableValue(value.Valid, value.Int16)}
}

func Int4(value pgtype.Int4) int4Builder {
	return int4Builder{value: newNullableValue(value.Valid, value.Int32)}
}

func Int8(value pgtype.Int8) int8Builder {
	return int8Builder{value: newNullableValue(value.Valid, value.Int64)}
}

func Float8(value pgtype.Float8) float8Builder {
	return float8Builder{value: newNullableValue(value.Valid, value.Float64)}
}

func Date(value pgtype.Date) dateBuilder {
	return dateBuilder{value: newNullableValue(finiteDate(value), value.Time)}
}

func Timestamp(value pgtype.Timestamp) timestampBuilder {
	return timestampBuilder{value: newNullableValue(finiteTimestamp(value), value.Time)}
}

func Timestamptz(value pgtype.Timestamptz) timestamptzBuilder {
	return timestamptzBuilder{value: newNullableValue(finiteTimestamptz(value), value.Time)}
}

func UUID(value pgtype.UUID) uuidBuilder {
	return uuidBuilder{value: newNullableValue(value.Valid, uuid.UUID(value.Bytes))}
}

func Numeric(value pgtype.Numeric) numericBuilder {
	return numericBuilder{value: value}
}
