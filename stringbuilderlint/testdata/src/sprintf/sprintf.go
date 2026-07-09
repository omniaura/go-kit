package sprintf

import "fmt"

type payload struct {
	Name string
}

func primitive(name string, count int, enabled bool) string {
	return fmt.Sprintf("%s/%d/%t", name, count, enabled) // want "fmt.Sprintf with primitive arguments should use strings.Builder and strconv"
}

func floatPrimitive(name string, ratio float64) string {
	return fmt.Sprintf("%s=%.2f", name, ratio) // want "fmt.Sprintf with primitive arguments should use strings.Builder and strconv"
}

func unsupportedStruct(value payload) string {
	return fmt.Sprintf("%v", value)
}

func unsupportedWidth(name string) string {
	return fmt.Sprintf("%10s", name)
}
