package pgdecode

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

type textDecoder struct {
	value pgtype.Text
}

func Text(value pgtype.Text) textDecoder {
	return textDecoder{value: value}
}

func (d textDecoder) Value() string {
	return d.value.String
}

func (d textDecoder) Fill(ptr *string) {
	if d.value.Valid {
		*ptr = d.value.String
	}
}

type timeDecoder struct {
	value time.Time
	valid bool
}

func Timestamptz(value pgtype.Timestamptz) timeDecoder {
	return timeDecoder{value: value.Time, valid: value.Valid}
}

func (d timeDecoder) Value() time.Time {
	return d.value
}

func (d timeDecoder) Fill(ptr *time.Time) {
	if d.valid {
		*ptr = d.value
	}
}

type uuidDecoder struct {
	value pgtype.UUID
}

func UUID(value pgtype.UUID) uuidDecoder {
	return uuidDecoder{value: value}
}

func (d uuidDecoder) Value() [16]byte {
	return d.value.Bytes
}
