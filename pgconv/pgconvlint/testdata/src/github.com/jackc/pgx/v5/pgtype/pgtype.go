package pgtype

import "time"

type Text struct {
	String string
	Valid  bool
}

type Bool struct {
	Bool  bool
	Valid bool
}

type Int2 struct {
	Int16 int16
	Valid bool
}

type Int4 struct {
	Int32 int32
	Valid bool
}

type Int8 struct {
	Int64 int64
	Valid bool
}

type Float8 struct {
	Float64 float64
	Valid   bool
}

type Date struct {
	Time  time.Time
	Valid bool
}

type Timestamp struct {
	Time  time.Time
	Valid bool
}

type Timestamptz struct {
	Time  time.Time
	Valid bool
}

type UUID struct {
	Bytes [16]byte
	Valid bool
}
