package sqlconv

import (
	"database/sql"
	"time"
)

func NString[S ~string](s S) sql.NullString {
	return sql.NullString{String: string(s), Valid: s != ""}
}

func NBool[B ~bool](b B) sql.NullBool {
	return sql.NullBool{Bool: bool(b), Valid: true}
}

func NInt64[I ~int64 | ~int](i I) sql.NullInt64 {
	return sql.NullInt64{Int64: int64(i), Valid: true}
}

func NTime(t time.Time) sql.NullTime {
	return sql.NullTime{Time: t, Valid: !t.IsZero()}
}

func NTimeUnix[T ~int64](t T) sql.NullTime {
	return sql.NullTime{Time: time.Unix(int64(t), 0), Valid: t > 0}
}

func FillNullString[S ~string](value sql.NullString, ptr *S) {
	if value.Valid {
		*ptr = S(value.String)
	}
}

func FillNullBool[B ~bool](value sql.NullBool, ptr *B) {
	if value.Valid {
		*ptr = B(value.Bool)
	}
}

func FillNullInt32[I ~int32 | ~int](value sql.NullInt32, ptr *I) {
	if value.Valid {
		*ptr = I(value.Int32)
	}
}

func FillNullInt64[I ~int64 | ~int](value sql.NullInt64, ptr *I) {
	if value.Valid {
		*ptr = I(value.Int64)
	}
}

func FillNullTime(value sql.NullTime, ptr *time.Time) {
	if value.Valid {
		*ptr = value.Time
	}
}
