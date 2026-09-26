package errs

import "encoding/json"

// JSON renders the Public view as application/json:
//
//	{"message":"…","code":"…","ref":"…","status":500,"action":"…","retryable":true,…}
var JSON Encoder = jsonEncoder{}

type jsonEncoder struct{}

func (jsonEncoder) ContentType() string { return "application/json" }

func (jsonEncoder) Encode(p Public) ([]byte, error) {
	b, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// ProblemJSON renders RFC 9457 problem details (application/problem+json).
// The code becomes the problem type (TypeBase + code, or about:blank when
// TypeBase is empty), the message is the title, and the rest of the Public
// view rides as extension members.
var ProblemJSON Encoder = ProblemEncoder{}

// ProblemEncoder is ProblemJSON with a configurable type URI prefix.
type ProblemEncoder struct {
	// TypeBase, e.g. "https://docs.example.com/errors/", is prefixed to the
	// code to form the problem type URI.
	TypeBase string
}

func (ProblemEncoder) ContentType() string { return "application/problem+json" }

func (p ProblemEncoder) Encode(pub Public) ([]byte, error) {
	typ := "about:blank"
	if p.TypeBase != "" {
		typ = p.TypeBase + pub.Code
	}
	b, err := json.Marshal(struct {
		Fields       map[string]string `json:"fields,omitempty"`
		Params       map[string]any    `json:"params,omitempty"`
		Type         string            `json:"type"`
		Title        string            `json:"title"`
		Code         string            `json:"code"`
		Ref          string            `json:"ref,omitempty"`
		Action       Action            `json:"action,omitempty"`
		RetryAfterMs int64             `json:"retryAfterMs,omitempty"`
		Status       int               `json:"status"`
		Retryable    bool              `json:"retryable,omitempty"`
	}{
		Fields:       pub.Fields,
		Params:       pub.Params,
		Type:         typ,
		Title:        pub.Message,
		Code:         pub.Code,
		Ref:          pub.Ref,
		Action:       pub.Action,
		RetryAfterMs: pub.RetryAfterMs,
		Status:       pub.Status,
		Retryable:    pub.Retryable,
	})
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}
