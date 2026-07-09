//lint:file-ignore uncheckederrlint fixture verifies file suppression
package fileignore

import "encoding/json"

func ignoredFile(data []byte) {
	var value map[string]any
	_ = json.Unmarshal(data, &value)
}
