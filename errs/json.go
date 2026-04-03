package errs

import (
	"encoding/json"
	"unique"
)

type problemJSON struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail,omitempty"`
}

func (e *Error) problemJSON() problemJSON {
	return problemJSON{
		Type:   "about:blank",
		Title:  e.Title(),
		Status: int(e.Status),
		Detail: e.Message(),
	}
}

func (e *Error) MarshalJSON() ([]byte, error) {
	return json.Marshal(e.problemJSON())
}

func (e errorMessage) MarshalJSON() ([]byte, error) {
	return json.Marshal(e.h.Value())
}

func (e *errorMessage) UnmarshalJSON(data []byte) error {
	var msg string
	if err := json.Unmarshal(data, &msg); err != nil {
		return err
	}
	*e = errorMessage{h: unique.Make(msg)}
	return nil
}
