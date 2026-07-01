package pgencode

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

type textBuilder struct {
	value string
}

func String(value string) textBuilder {
	return textBuilder{value: value}
}

func StringPtr(value *string) textBuilder {
	if value == nil {
		return textBuilder{}
	}
	return textBuilder{value: *value}
}

func (b textBuilder) EmptyIsNull() textBuilder {
	return b
}

func (b textBuilder) Text() pgtype.Text {
	return pgtype.Text{String: b.value, Valid: true}
}

type boolBuilder struct {
	value bool
}

func Bool(value bool) boolBuilder {
	return boolBuilder{value: value}
}

func (b boolBuilder) Bool() pgtype.Bool {
	return pgtype.Bool{Bool: b.value, Valid: true}
}

type int64Builder struct {
	value int64
}

func Int64(value int64) int64Builder {
	return int64Builder{value: value}
}

func (b int64Builder) Int8() pgtype.Int8 {
	return pgtype.Int8{Int64: b.value, Valid: true}
}

type timeBuilder struct {
	value time.Time
}

func Time(value time.Time) timeBuilder {
	return timeBuilder{value: value}
}

func (b timeBuilder) Timestamptz() pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: b.value, Valid: true}
}

type uuidBuilder struct {
	value [16]byte
}

func UUID(value [16]byte) uuidBuilder {
	return uuidBuilder{value: value}
}

func (b uuidBuilder) UUID() pgtype.UUID {
	return pgtype.UUID{Bytes: b.value, Valid: true}
}
