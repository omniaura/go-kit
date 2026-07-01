package imports

import "github.com/jackc/pgx/v5/pgtype"

func encode(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: true} // want "manual pgtype.Text encode; use pgencode.String"
}
