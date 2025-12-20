package errs

import (
	"bytes"
	"encoding/json"
	"strconv"
	"unique"
)

func (e *Error) marshalJSONBuffer(buf *bytes.Buffer) {
	const json1 = `{"message":"`
	const json2 = `","status":`
	const json3 = `}`
	msg := e.Message()
	msglen := len(msg)
	for i := range e.rspAnnotations {
		msglen += len(e.rspAnnotations[i]) + 2
	}
	status := strconv.Itoa(int(e.Status))
	buf.Grow(len(json1) + msglen + len(json2) + len(status) + len(json3))
	buf.WriteString(json1)
	buf.WriteString(msg)
	for i := range e.rspAnnotations {
		buf.WriteString(": ")
		buf.WriteString(e.rspAnnotations[i])
	}
	buf.WriteString(json2)
	buf.WriteString(status)
	buf.WriteString(json3)
}

func (e *Error) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	e.marshalJSONBuffer(&buf)
	return buf.Bytes(), nil
}

func (e errorMessage) MarshalJSON() ([]byte, error) {
	str := e.h.Value()
	var buf bytes.Buffer
	buf.Grow(len(str) + 2)
	buf.WriteString(`"`)
	buf.WriteString(str)
	buf.WriteString(`"`)
	return buf.Bytes(), nil
}

func (e *errorMessage) UnmarshalJSON(data []byte) error {
	var msg string
	if err := json.Unmarshal(data, &msg); err != nil {
		return err
	}
	*e = errorMessage{h: unique.Make(msg)}
	return nil
}
