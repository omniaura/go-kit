package imports

import "github.com/jackc/pgx/v5/pgtype"

func decode(txt pgtype.Text) string {
	if txt.Valid {
		return txt.String // want "manual pgtype.Text decode; use pgdecode.Text"
	}
	return ""
}
