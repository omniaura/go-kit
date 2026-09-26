package errs

import "github.com/rs/zerolog"

// Attr is a server-only key/value: logged and recorded, never sent to clients.
type Attr struct {
	Value any
	Key   string
}

// String creates a string attribute.
func String(key, value string) Attr { return Attr{Key: key, Value: value} }

// Int creates an int attribute.
func Int(key string, value int) Attr { return Attr{Key: key, Value: value} }

// Int64 creates an int64 attribute.
func Int64(key string, value int64) Attr { return Attr{Key: key, Value: value} }

// Bool creates a bool attribute.
func Bool(key string, value bool) Attr { return Attr{Key: key, Value: value} }

// Any creates an attribute with any value.
func Any(key string, value any) Attr { return Attr{Key: key, Value: value} }

func applyAttrs(e *zerolog.Event, attrs []Attr) *zerolog.Event {
	for _, attr := range attrs {
		switch v := attr.Value.(type) {
		case string:
			e = e.Str(attr.Key, v)
		case int:
			e = e.Int(attr.Key, v)
		case int64:
			e = e.Int64(attr.Key, v)
		case float64:
			e = e.Float64(attr.Key, v)
		case bool:
			e = e.Bool(attr.Key, v)
		case error:
			e = e.AnErr(attr.Key, v)
		case []string:
			e = e.Strs(attr.Key, v)
		default:
			e = e.Interface(attr.Key, v)
		}
	}
	return e
}
