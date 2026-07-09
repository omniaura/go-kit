package a

func twoParts(a, b string) string {
	return a + b
}

func threeParts(a, b, c string) string {
	return a + ":" + b + ":" + c
}

func nested(a, b, c string) string {
	return (a + b) + c
}

func fiveDynamic(a, b, c, d, e string) string {
	return a + ":" + b + ":" + c + ":" + d + ":" + e
}

func thirtyTwoDynamic(a string, b string, c string, d string, e string, f string, g string, h string, i string, j string, k string, l string, m string, n string, o string, p string, q string, r string, s string, t string, u string, v string, w string, x string, y string, z string, aa string, ab string, ac string, ad string, ae string, af string) string {
	return a + b + c + d + e + f + g + h + i + j + k + l + m + n + o + p + q + r + s + t + u + v + w + x + y + z + aa + ab + ac + ad + ae + af // want "string concatenation with 32 dynamic parts should use strings.Builder"
}

func constantOnly() string {
	return "a" + "b" + "c"
}

func ignored(a, b, c string) string {
	//lint:ignore stringbuilderlint fixture verifies line suppression
	return a + b + c
}
