package a

import (
	"encoding/json"
	"strings"

	"golang.org/x/sync/errgroup"
)

type store struct{}

func (store) CreateThing() (int, error) {
	return 0, nil
}

func (store) ReadThing() (int, error) {
	return 0, nil
}

func ignoredUnmarshal(data []byte) {
	var value map[string]any
	_ = json.Unmarshal(data, &value) // want "discarded JSON encode/decode error should be handled or logged"
}

func nakedUnmarshal(data []byte) {
	var value map[string]any
	json.Unmarshal(data, &value) // want "discarded JSON encode/decode error should be handled or logged"
}

func ignoredMarshal(value map[string]any) {
	_, _ = json.Marshal(value) // want "discarded JSON encode/decode error should be handled or logged"
}

func nakedMarshal(value map[string]any) {
	json.Marshal(value) // want "discarded JSON encode/decode error should be handled or logged"
}

func ignoredDecode(raw string) {
	var value map[string]any
	_ = json.NewDecoder(strings.NewReader(raw)).Decode(&value) // want "discarded JSON encode/decode error should be handled or logged"
}

func ignoredEncode(value map[string]any) {
	var out strings.Builder
	_ = json.NewEncoder(&out).Encode(value) // want "discarded JSON encode/decode error should be handled or logged"
}

func ignoredErrgroupWait() {
	var group errgroup.Group
	_ = group.Wait() // want "discarded errgroup.Wait error should be handled or use sync.WaitGroup when goroutines cannot fail"
}

func ignoredWritePath(s store) {
	_, _ = s.CreateThing() // want "discarded write-path error should be handled or logged"
}

func handled(data []byte, s store) error {
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	_, err := s.CreateThing()
	return err
}

func readOnlyIgnored(s store) {
	_, _ = s.ReadThing()
}

func ignoredWithComment(data []byte) {
	var value map[string]any
	//lint:ignore uncheckederrlint fixture verifies line suppression
	_ = json.Unmarshal(data, &value)
}
