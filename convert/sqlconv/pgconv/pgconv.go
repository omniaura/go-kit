package pgconv

import (
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type signedInt interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64
}

type floatNumber interface {
	~float32 | ~float64
}

func valueOr[T any](valid bool, value, fallback T) T {
	if valid {
		return value
	}
	return fallback
}

func ptrValue[T any](valid bool, value T) *T {
	if !valid {
		return nil
	}
	v := value
	return &v
}

func fillValue[T any](valid bool, value T, ptr *T) {
	if valid {
		*ptr = value
	}
}

func Text[S ~string](s S) pgtype.Text {
	return pgtype.Text{String: string(s), Valid: true}
}

func NText[S ~string](s S) pgtype.Text {
	return pgtype.Text{String: string(s), Valid: s != ""}
}

func TextFromPtr[S ~string](value *S) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return Text(*value)
}

func TextValue[S ~string](value pgtype.Text) S {
	return TextOr(value, S(""))
}

func TextOr[S ~string](value pgtype.Text, fallback S) S {
	return valueOr(value.Valid, S(value.String), fallback)
}

func TextPtr(value pgtype.Text) *string {
	return ptrValue(value.Valid, value.String)
}

func FillText[S ~string](value pgtype.Text, ptr *S) {
	fillValue(value.Valid, S(value.String), ptr)
}

func FillTextOr[S ~string](value pgtype.Text, ptr *S, fallback S) {
	*ptr = TextOr(value, fallback)
}

func NBool[B ~bool](b B) pgtype.Bool {
	return pgtype.Bool{Bool: bool(b), Valid: true}
}

func BoolFromPtr[B ~bool](value *B) pgtype.Bool {
	if value == nil {
		return pgtype.Bool{}
	}
	return NBool(*value)
}

func BoolValue[B ~bool](value pgtype.Bool) B {
	return BoolOr(value, B(false))
}

func BoolOr[B ~bool](value pgtype.Bool, fallback B) B {
	return valueOr(value.Valid, B(value.Bool), fallback)
}

func BoolPtr(value pgtype.Bool) *bool {
	return ptrValue(value.Valid, value.Bool)
}

func FillBool[B ~bool](value pgtype.Bool, ptr *B) {
	fillValue(value.Valid, B(value.Bool), ptr)
}

func FillBoolOr[B ~bool](value pgtype.Bool, ptr *B, fallback B) {
	*ptr = BoolOr(value, fallback)
}

func NInt2[I signedInt](i I) pgtype.Int2 {
	return pgtype.Int2{Int16: int16(i), Valid: true}
}

func Int2FromPtr[I signedInt](value *I) pgtype.Int2 {
	if value == nil {
		return pgtype.Int2{}
	}
	return NInt2(*value)
}

func Int2Value[I signedInt](value pgtype.Int2) I {
	return Int2Or(value, I(0))
}

func Int2Or[I signedInt](value pgtype.Int2, fallback I) I {
	return valueOr(value.Valid, I(value.Int16), fallback)
}

func Int2Ptr[I signedInt](value pgtype.Int2) *I {
	return ptrValue(value.Valid, I(value.Int16))
}

func FillInt2[I signedInt](value pgtype.Int2, ptr *I) {
	fillValue(value.Valid, I(value.Int16), ptr)
}

func FillInt2Or[I signedInt](value pgtype.Int2, ptr *I, fallback I) {
	*ptr = Int2Or(value, fallback)
}

func NInt4[I signedInt](i I) pgtype.Int4 {
	return pgtype.Int4{Int32: int32(i), Valid: true}
}

func Int4FromPtr[I signedInt](value *I) pgtype.Int4 {
	if value == nil {
		return pgtype.Int4{}
	}
	return NInt4(*value)
}

func Int4Value[I signedInt](value pgtype.Int4) I {
	return Int4Or(value, I(0))
}

func Int4Or[I signedInt](value pgtype.Int4, fallback I) I {
	return valueOr(value.Valid, I(value.Int32), fallback)
}

func Int4Ptr[I signedInt](value pgtype.Int4) *I {
	return ptrValue(value.Valid, I(value.Int32))
}

func FillInt4[I signedInt](value pgtype.Int4, ptr *I) {
	fillValue(value.Valid, I(value.Int32), ptr)
}

func FillInt4Or[I signedInt](value pgtype.Int4, ptr *I, fallback I) {
	*ptr = Int4Or(value, fallback)
}

func NInt8[I signedInt](i I) pgtype.Int8 {
	return pgtype.Int8{Int64: int64(i), Valid: true}
}

func Int8FromPtr[I signedInt](value *I) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return NInt8(*value)
}

func Int8Value[I signedInt](value pgtype.Int8) I {
	return Int8Or(value, I(0))
}

func Int8Or[I signedInt](value pgtype.Int8, fallback I) I {
	return valueOr(value.Valid, I(value.Int64), fallback)
}

func Int8Ptr[I signedInt](value pgtype.Int8) *I {
	return ptrValue(value.Valid, I(value.Int64))
}

func FillInt8[I signedInt](value pgtype.Int8, ptr *I) {
	fillValue(value.Valid, I(value.Int64), ptr)
}

func FillInt8Or[I signedInt](value pgtype.Int8, ptr *I, fallback I) {
	*ptr = Int8Or(value, fallback)
}

func NFloat8[F floatNumber](f F) pgtype.Float8 {
	return pgtype.Float8{Float64: float64(f), Valid: true}
}

func Float8FromPtr[F floatNumber](value *F) pgtype.Float8 {
	if value == nil {
		return pgtype.Float8{}
	}
	return NFloat8(*value)
}

func Float8Value[F floatNumber](value pgtype.Float8) F {
	return Float8Or(value, F(0))
}

func Float8Or[F floatNumber](value pgtype.Float8, fallback F) F {
	return valueOr(value.Valid, F(value.Float64), fallback)
}

func Float8Ptr[F floatNumber](value pgtype.Float8) *F {
	return ptrValue(value.Valid, F(value.Float64))
}

func FillFloat8[F floatNumber](value pgtype.Float8, ptr *F) {
	fillValue(value.Valid, F(value.Float64), ptr)
}

func FillFloat8Or[F floatNumber](value pgtype.Float8, ptr *F, fallback F) {
	*ptr = Float8Or(value, fallback)
}

func NDate(t time.Time) pgtype.Date {
	return pgtype.Date{Time: t, Valid: !t.IsZero()}
}

func DateFromPtr(value *time.Time) pgtype.Date {
	if value == nil {
		return pgtype.Date{}
	}
	return NDate(*value)
}

func DateValue(value pgtype.Date) time.Time {
	return DateOr(value, time.Time{})
}

func DateOr(value pgtype.Date, fallback time.Time) time.Time {
	return valueOr(value.Valid, value.Time, fallback)
}

func DatePtr(value pgtype.Date) *time.Time {
	return ptrValue(value.Valid, value.Time)
}

func FillDate(value pgtype.Date, ptr *time.Time) {
	fillValue(value.Valid, value.Time, ptr)
}

func FillDateOr(value pgtype.Date, ptr *time.Time, fallback time.Time) {
	*ptr = DateOr(value, fallback)
}

func NTimestamp(t time.Time) pgtype.Timestamp {
	return pgtype.Timestamp{Time: t, Valid: !t.IsZero()}
}

func TimestampFromPtr(value *time.Time) pgtype.Timestamp {
	if value == nil {
		return pgtype.Timestamp{}
	}
	return NTimestamp(*value)
}

func TimestampValue(value pgtype.Timestamp) time.Time {
	return TimestampOr(value, time.Time{})
}

func TimestampOr(value pgtype.Timestamp, fallback time.Time) time.Time {
	return valueOr(value.Valid, value.Time, fallback)
}

func TimestampPtr(value pgtype.Timestamp) *time.Time {
	return ptrValue(value.Valid, value.Time)
}

func FillTimestamp(value pgtype.Timestamp, ptr *time.Time) {
	fillValue(value.Valid, value.Time, ptr)
}

func FillTimestampOr(value pgtype.Timestamp, ptr *time.Time, fallback time.Time) {
	*ptr = TimestampOr(value, fallback)
}

func NTimestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: !t.IsZero()}
}

func TimestamptzFromPtr(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return NTimestamptz(*value)
}

func TimestamptzValue(value pgtype.Timestamptz) time.Time {
	return TimestamptzOr(value, time.Time{})
}

func TimestamptzOr(value pgtype.Timestamptz, fallback time.Time) time.Time {
	return valueOr(value.Valid, value.Time, fallback)
}

func TimestamptzPtr(value pgtype.Timestamptz) *time.Time {
	return ptrValue(value.Valid, value.Time)
}

func FillTimestamptz(value pgtype.Timestamptz, ptr *time.Time) {
	fillValue(value.Valid, value.Time, ptr)
}

func FillTimestamptzOr(value pgtype.Timestamptz, ptr *time.Time, fallback time.Time) {
	*ptr = TimestamptzOr(value, fallback)
}

func NUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte(id), Valid: id != uuid.Nil}
}

func UUIDFromPtr(value *uuid.UUID) pgtype.UUID {
	if value == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: [16]byte(*value), Valid: true}
}

func UUIDValue(value pgtype.UUID) uuid.UUID {
	return UUIDOr(value, uuid.Nil)
}

func UUIDOr(value pgtype.UUID, fallback uuid.UUID) uuid.UUID {
	return valueOr(value.Valid, uuid.UUID(value.Bytes), fallback)
}

func UUIDString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return uuid.UUID(value.Bytes).String()
}

func UUIDStringOr(value pgtype.UUID, fallback string) string {
	if !value.Valid {
		return fallback
	}
	return uuid.UUID(value.Bytes).String()
}

func UUIDPtr(value pgtype.UUID) *uuid.UUID {
	if !value.Valid {
		return nil
	}
	id := uuid.UUID(value.Bytes)
	return &id
}

func FillUUID(value pgtype.UUID, ptr *uuid.UUID) {
	if value.Valid {
		*ptr = uuid.UUID(value.Bytes)
	}
}

func FillUUIDOr(value pgtype.UUID, ptr *uuid.UUID, fallback uuid.UUID) {
	*ptr = UUIDOr(value, fallback)
}

func NumericFloat64(value pgtype.Numeric) (float64, error) {
	return NumericFloat64Or(value, 0)
}

func NumericFloat64Or(value pgtype.Numeric, fallback float64) (float64, error) {
	if !value.Valid {
		return fallback, nil
	}
	v, err := value.Float64Value()
	if err != nil {
		return fallback, err
	}
	return v.Float64, nil
}

func NumericFloat64Ptr(value pgtype.Numeric) (*float64, error) {
	if !value.Valid {
		return nil, nil
	}
	v, err := NumericFloat64(value)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func FillNumericFloat64(value pgtype.Numeric, ptr *float64) error {
	if !value.Valid {
		return nil
	}
	v, err := NumericFloat64(value)
	if err != nil {
		return err
	}
	*ptr = v
	return nil
}

func FillNumericFloat64Or(value pgtype.Numeric, ptr *float64, fallback float64) error {
	v, err := NumericFloat64Or(value, fallback)
	if err != nil {
		return err
	}
	*ptr = v
	return nil
}

func NumericInt64(value pgtype.Numeric) (int64, error) {
	return NumericInt64Or(value, 0)
}

func NumericInt64Or(value pgtype.Numeric, fallback int64) (int64, error) {
	if !value.Valid {
		return fallback, nil
	}
	v, err := value.Int64Value()
	if err != nil {
		return fallback, err
	}
	return v.Int64, nil
}

func NumericInt64Ptr(value pgtype.Numeric) (*int64, error) {
	if !value.Valid {
		return nil, nil
	}
	v, err := NumericInt64(value)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func FillNumericInt64(value pgtype.Numeric, ptr *int64) error {
	if !value.Valid {
		return nil
	}
	v, err := NumericInt64(value)
	if err != nil {
		return err
	}
	*ptr = v
	return nil
}

func FillNumericInt64Or(value pgtype.Numeric, ptr *int64, fallback int64) error {
	v, err := NumericInt64Or(value, fallback)
	if err != nil {
		return err
	}
	*ptr = v
	return nil
}
