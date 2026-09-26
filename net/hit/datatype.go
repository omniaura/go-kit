package hit

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/url"
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
	// map[string]string and map[string][]string (or pointers to them) and
	// unmarshals into *url.Values or *map[string]string.
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
		return nil, fmt.Errorf("hit: Form cannot marshal %T", v)
	}
	return []byte(values.Encode()), nil
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
