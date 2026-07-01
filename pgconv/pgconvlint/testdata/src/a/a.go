package a

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/omniaura/go-kit/pgconv/pgdecode"
	"github.com/omniaura/go-kit/pgconv/pgencode"
)

func encode(s string, p *string, b bool, n int64, tm time.Time, id [16]byte) {
	_ = pgtype.Text{String: s, Valid: true}       // want "manual pgtype.Text encode; use pgencode.String"
	_ = pgtype.Text{String: s, Valid: s != ""}    // want "manual pgtype.Text encode; use pgencode.String\\(\\.\\.\\.\\).EmptyIsNull\\(\\).Text\\(\\)"
	_ = pgtype.Text{String: *p, Valid: p != nil}  // want "manual pgtype.Text encode; use pgencode.StringPtr"
	_ = pgtype.Bool{Bool: b, Valid: true}         // want "manual pgtype.Bool encode; use pgencode.Bool"
	_ = pgtype.Int8{Int64: n, Valid: true}        // want "manual pgtype.Int8 encode; use pgencode.Int64"
	_ = pgtype.Timestamptz{Time: tm, Valid: true} // want "manual pgtype.Timestamptz encode; use pgencode.Time"
	_ = pgtype.UUID{Bytes: id, Valid: true}       // want "manual pgtype.UUID encode; use pgencode.UUID"
	_ = pgtype.Text{String: s}                    // want "manual pgtype.Text encode; use pgencode.String"
	//lint:ignore pgconvlint legacy test fixture
	_ = pgtype.Text{String: s, Valid: true}
	_ = pgtype.Text{}             // zero/null literals are allowed
	_ = pgtype.Int8{Valid: false} // explicit null literals are allowed
}

func decode(txt pgtype.Text, ts pgtype.Timestamptz, id pgtype.UUID) {
	var out struct {
		Name string
		Time time.Time
		ID   [16]byte
	}
	if txt.Valid {
		out.Name = txt.String // want "manual pgtype.Text decode; use pgdecode.Text"
	}
	if ts.Valid {
		out.Time = ts.Time // want "manual pgtype.Timestamptz decode; use pgdecode.Timestamptz"
	}
	if id.Valid {
		out.ID = id.Bytes // want "manual pgtype.UUID decode; use pgdecode.UUID"
	}

	_ = struct {
		CreatedAt time.Time
	}{CreatedAt: ts.Time}
}

func textValue(txt pgtype.Text) string {
	if txt.Valid {
		return txt.String // want "manual pgtype.Text decode; use pgdecode.Text"
	}
	return ""
}

func assign(txt pgtype.Text, value string) {
	txt.String = value // want "manual pgtype.Text field assignment; build the value with pgencode"
	txt.Valid = true
}

func pgText(s string) pgtype.Text { // want "tiny pgencode wrapper pgText; call pgencode directly at the use site"
	return pgencode.String(s).EmptyIsNull().Text()
}

func pgBool(b bool) pgtype.Bool { // want "tiny pgencode wrapper pgBool; call pgencode directly at the use site"
	return pgencode.Bool(b).Bool()
}

type wrappedTime struct {
	Time  time.Time
	Valid bool
}

func toPgTimestamptz(t wrappedTime) pgtype.Timestamptz { // want "tiny pgencode wrapper toPgTimestamptz; call pgencode directly at the use site"
	if !t.Valid {
		return pgtype.Timestamptz{}
	}
	return pgencode.Time(t.Time).Timestamptz()
}

func decodeText(t pgtype.Text) string { // want "tiny pgdecode wrapper decodeText; call pgdecode directly at the use site"
	return pgdecode.Text(t).Value()
}

//lint:ignore pgconvlint legacy wrapper kept for compatibility
func ignoredDecodeText(t pgtype.Text) string {
	return pgdecode.Text(t).Value()
}

func usesWrappers(s string, b bool, t pgtype.Text) {
	_ = pgText(s)     // want "call to tiny pgencode wrapper pgText; use pgencode directly"
	_ = pgBool(b)     // want "call to tiny pgencode wrapper pgBool; use pgencode directly"
	_ = decodeText(t) // want "call to tiny pgdecode wrapper decodeText; use pgdecode directly"
}

type row struct {
	Name pgtype.Text
}

type response struct {
	Name string
}

func mapper(r row) response {
	return response{Name: pgdecode.Text(r.Name).Value()}
}
