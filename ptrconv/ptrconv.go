package ptrconv

func Str[S ~string](s S) *S { return &s }
func Bool[B ~bool](b B) *B  { return &b }
