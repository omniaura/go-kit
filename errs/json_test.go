package errs

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestErrorMarshalJSON_ProblemJSON(t *testing.T) {
	t.Parallel()

	err := NewFactory(http.StatusBadRequest, "invalid request").
		New(context.Background()).
		Err(errors.New(`bad "value"`))

	data, marshalErr := json.Marshal(err)
	if marshalErr != nil {
		t.Fatalf("MarshalJSON() error = %v", marshalErr)
	}

	var got map[string]any
	if unmarshalErr := json.Unmarshal(data, &got); unmarshalErr != nil {
		t.Fatalf("json.Unmarshal() error = %v", unmarshalErr)
	}

	if got["type"] != "about:blank" {
		t.Fatalf("type = %v, want about:blank", got["type"])
	}
	if got["title"] != http.StatusText(http.StatusBadRequest) {
		t.Fatalf("title = %v, want %q", got["title"], http.StatusText(http.StatusBadRequest))
	}
	if got["detail"] != `invalid request: bad "value"` {
		t.Fatalf("detail = %v, want %q", got["detail"], `invalid request: bad "value"`)
	}
	if got["status"] != float64(http.StatusBadRequest) {
		t.Fatalf("status = %v, want %d", got["status"], http.StatusBadRequest)
	}
}

func TestErrorAbort_WritesProblemJSONResponse(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	err := NewFactory(http.StatusNotFound, "resource not found").
		New(context.Background()).
		Err(errors.New("record does not exist"))

	if !err.Abort(recorder) {
		t.Fatal("Abort() = false, want true")
	}

	response := recorder.Result()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("status code = %d, want %d", response.StatusCode, http.StatusNotFound)
	}
	if got := response.Header.Get("Content-Type"); got != "application/problem+json" {
		t.Fatalf("Content-Type = %q, want %q", got, "application/problem+json")
	}

	var body map[string]any
	if decodeErr := json.NewDecoder(response.Body).Decode(&body); decodeErr != nil {
		t.Fatalf("Decode() error = %v", decodeErr)
	}

	if body["title"] != http.StatusText(http.StatusNotFound) {
		t.Fatalf("title = %v, want %q", body["title"], http.StatusText(http.StatusNotFound))
	}
	if body["detail"] != "resource not found: record does not exist" {
		t.Fatalf("detail = %v, want %q", body["detail"], "resource not found: record does not exist")
	}
}

func TestNilErrorAbort_DoesNothing(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	var err *Error

	if err.Abort(recorder) {
		t.Fatal("Abort() = true, want false")
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", recorder.Code, http.StatusOK)
	}
	if recorder.Body.Len() != 0 {
		t.Fatalf("body length = %d, want 0", recorder.Body.Len())
	}
}
