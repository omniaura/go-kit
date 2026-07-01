//lint:file-ignore pgconvlint legacy file fixture
package fileignore

import "github.com/jackc/pgx/v5/pgtype"

func encode(s string) {
	_ = pgtype.Text{String: s, Valid: true}
}
