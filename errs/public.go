package errs

import "time"

// Public is the client view of an error: everything in it may be shown to the
// person who made the request. Encoders render it; non-HTTP transports put it
// in their frames.
type Public struct {
	Fields       map[string]string `json:"fields,omitempty"`
	Params       map[string]any    `json:"params,omitempty"`
	Message      string            `json:"message"`
	Code         string            `json:"code"`
	Ref          string            `json:"ref,omitempty"`
	Action       Action            `json:"action,omitempty"`
	RetryAfterMs int64             `json:"retryAfterMs,omitempty"`
	Status       int               `json:"status"`
	Retryable    bool              `json:"retryable,omitempty"`
}

// Public returns the client view of e.
func (e *Error) Public() Public {
	return Public{
		Fields:       e.fields,
		Params:       e.params,
		Message:      e.Message(),
		Code:         e.code,
		Ref:          e.Ref(),
		Action:       e.action,
		RetryAfterMs: e.retryAfter.Milliseconds(),
		Status:       int(e.Status),
		Retryable:    e.Retryable(),
	}
}

// RetryAfterDuration returns RetryAfterMs as a duration.
func (p Public) RetryAfterDuration() time.Duration {
	return time.Duration(p.RetryAfterMs) * time.Millisecond
}
