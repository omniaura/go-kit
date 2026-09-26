package hit

import (
	"encoding"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/url"
	"reflect"
	"strconv"
	"strings"
)

// DataType is a wire encoding for request and response bodies: the
// Content-Type it announces and how values are marshalled to and from it.
// JSON is the default; XML, Form, Text and Bytes are built in, and any other
// encoding (protobuf, msgpack, CBOR, ...) is one small implementation away.
type DataType interface {
	ContentType() string
	Marshal(v any) ([]byte, error)
	Unmarshal(data []byte, v any) error
}

// HasDataType is implemented by body types that are not JSON. Body, Do and
// error decoding check the pointer they are given, so a type declares its
// encoding once and every call site stays typed and inferred:
//
//	type Envelope struct{ XMLName xml.Name `xml:"Envelope"`; ... }
//	func (*Envelope) DataType() hit.DataType { return hit.XML }
//
//	soap.POST("/service").Body(&env).Do(ctx, &reply) // XML both ways
//
// For types you do not own (url.Values, a generated struct) use BodyAs /
// ResponseAs, or set a default for a whole SDK with Client.DataType.
type HasDataType interface {
	DataType() DataType
}

var (
	// JSON is application/json via encoding/json.
	JSON DataType = jsonType{}
	// XML is application/xml via encoding/xml.
	XML DataType = xmlType{}
	// Form is application/x-www-form-urlencoded. It marshals url.Values,
	// map[string]string, map[string][]string and structs (or pointers to
	// them), and unmarshals into *url.Values or *map[string]string. Struct
	// fields are named by a `form:"name,omitempty"` tag, falling back to the
	// json tag's name, then the field name; "-" skips a field. Supported
	// field types: strings, bools, integers, floats, encoding.TextMarshaler,
	// pointers to those (nil is omitted), slices of those (repeated keys) and
	// embedded structs (flattened).
	Form DataType = formType{}
	// Text is text/plain; charset=utf-8 for string and []byte.
	Text DataType = rawType{contentType: "text/plain; charset=utf-8"}
	// Bytes is application/octet-stream for string and []byte.
	Bytes DataType = rawType{contentType: "application/octet-stream"}
)

// dataTypeOf returns v's own DataType when it implements HasDataType, else
// fallback.
func dataTypeOf(v any, fallback DataType) DataType {
	if h, ok := v.(HasDataType); ok {
		if dt := h.DataType(); dt != nil {
			return dt
		}
	}
	return fallback
}

type jsonType struct{}

func (jsonType) ContentType() string                { return "application/json" }
func (jsonType) Marshal(v any) ([]byte, error)      { return json.Marshal(v) }
func (jsonType) Unmarshal(data []byte, v any) error { return json.Unmarshal(data, v) }

type xmlType struct{}

func (xmlType) ContentType() string                { return "application/xml" }
func (xmlType) Marshal(v any) ([]byte, error)      { return xml.Marshal(v) }
func (xmlType) Unmarshal(data []byte, v any) error { return xml.Unmarshal(data, v) }

type formType struct{}

func (formType) ContentType() string { return "application/x-www-form-urlencoded" }

func (formType) Marshal(v any) ([]byte, error) {
	var values url.Values
	switch t := v.(type) {
	case url.Values:
		values = t
	case *url.Values:
		values = *t
	case map[string][]string:
		values = t
	case *map[string][]string:
		values = *t
	case map[string]string:
		values = mapValues(t)
	case *map[string]string:
		values = mapValues(*t)
	default:
		var err error
		values, err = structValues(v)
		if err != nil {
			return nil, err
		}
	}
	return []byte(values.Encode()), nil
}

// structValues encodes a struct (or pointer to one) as form values.
func structValues(v any) (url.Values, error) {
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return url.Values{}, nil
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil, fmt.Errorf("hit: Form cannot marshal %T", v)
	}
	values := url.Values{}
	if err := addStructValues(values, rv); err != nil {
		return nil, err
	}
	return values, nil
}

func addStructValues(values url.Values, rv reflect.Value) error {
	rt := rv.Type()
	for i := range rt.NumField() {
		field := rt.Field(i)
		fv := rv.Field(i)
		if field.Anonymous && fv.Kind() == reflect.Struct && field.Tag.Get("form") == "" {
			if err := addStructValues(values, fv); err != nil {
				return err
			}
			continue
		}
		if !field.IsExported() {
			continue
		}
		name, omitEmpty := formFieldName(field)
		if name == "-" {
			continue
		}
		if omitEmpty && fv.IsZero() {
			continue
		}
		if fv.Kind() == reflect.Slice && fv.Type().Elem().Kind() != reflect.Uint8 {
			for j := range fv.Len() {
				s, ok, err := formScalar(fv.Index(j))
				if err != nil {
					return fmt.Errorf("hit: Form field %s: %w", field.Name, err)
				}
				if ok {
					values.Add(name, s)
				}
			}
			continue
		}
		s, ok, err := formScalar(fv)
		if err != nil {
			return fmt.Errorf("hit: Form field %s: %w", field.Name, err)
		}
		if ok {
			values.Set(name, s)
		}
	}
	return nil
}

func formFieldName(field reflect.StructField) (string, bool) {
	tag, ok := field.Tag.Lookup("form")
	if !ok {
		tag = field.Tag.Get("json")
	}
	name, opts, _ := strings.Cut(tag, ",")
	if name == "" {
		name = field.Name
	}
	return name, strings.Contains(","+opts+",", ",omitempty,")
}

var textMarshalerType = reflect.TypeFor[encoding.TextMarshaler]()

// formScalar renders one value; ok=false means "omit" (a nil pointer).
func formScalar(fv reflect.Value) (string, bool, error) {
	for fv.Kind() == reflect.Pointer || fv.Kind() == reflect.Interface {
		if fv.IsNil() {
			return "", false, nil
		}
		if fv.Type().Implements(textMarshalerType) {
			break
		}
		fv = fv.Elem()
	}
	if fv.Type().Implements(textMarshalerType) {
		b, err := fv.Interface().(encoding.TextMarshaler).MarshalText()
		return string(b), err == nil, err
	}
	if fv.CanAddr() && reflect.PointerTo(fv.Type()).Implements(textMarshalerType) {
		b, err := fv.Addr().Interface().(encoding.TextMarshaler).MarshalText()
		return string(b), err == nil, err
	}
	switch fv.Kind() {
	case reflect.String:
		return fv.String(), true, nil
	case reflect.Bool:
		return strconv.FormatBool(fv.Bool()), true, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(fv.Int(), 10), true, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(fv.Uint(), 10), true, nil
	case reflect.Float32:
		return strconv.FormatFloat(fv.Float(), 'f', -1, 32), true, nil
	case reflect.Float64:
		return strconv.FormatFloat(fv.Float(), 'f', -1, 64), true, nil
	default:
		return "", false, fmt.Errorf("unsupported type %s", fv.Type())
	}
}

func (formType) Unmarshal(data []byte, v any) error {
	values, err := url.ParseQuery(string(data))
	if err != nil {
		return err
	}
	switch t := v.(type) {
	case *url.Values:
		*t = values
	case *map[string]string:
		m := make(map[string]string, len(values))
		for k := range values {
			m[k] = values.Get(k)
		}
		*t = m
	default:
		return fmt.Errorf("hit: Form cannot unmarshal into %T", v)
	}
	return nil
}

func mapValues(m map[string]string) url.Values {
	values := make(url.Values, len(m))
	for k, v := range m {
		values.Set(k, v)
	}
	return values
}

type rawType struct{ contentType string }

func (r rawType) ContentType() string { return r.contentType }

func (rawType) Marshal(v any) ([]byte, error) {
	switch t := v.(type) {
	case []byte:
		return t, nil
	case *[]byte:
		return *t, nil
	case string:
		return []byte(t), nil
	case *string:
		return []byte(*t), nil
	default:
		return nil, fmt.Errorf("hit: raw body cannot marshal %T", v)
	}
}

func (rawType) Unmarshal(data []byte, v any) error {
	switch t := v.(type) {
	case *[]byte:
		*t = append((*t)[:0], data...)
	case *string:
		*t = string(data)
	default:
		return fmt.Errorf("hit: raw body cannot unmarshal into %T", v)
	}
	return nil
}
