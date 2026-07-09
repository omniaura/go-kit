package a

func twoParts(a, b string) string {
	return a + b
}

func threeParts(a, b, c string) string {
	return a + ":" + b + ":" + c // want "string concatenation with 5 parts should use strings.Builder"
}

func nested(a, b, c string) string {
	return (a + b) + c // want "string concatenation with 3 parts should use strings.Builder"
}

func constantOnly() string {
	return "a" + "b" + "c"
}

func ignored(a, b, c string) string {
	//lint:ignore stringbuilderlint fixture verifies line suppression
	return a + b + c
}
