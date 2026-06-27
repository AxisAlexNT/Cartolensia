package database

import (
	"encoding/json"
	"math"
	"testing"
)

func TestMetadataOrEmptySanitizesNonFiniteFloats(t *testing.T) {
	metadata := metadataOrEmpty(map[string]any{
		"ok":   1.25,
		"nan":  math.NaN(),
		"inf":  math.Inf(1),
		"list": []any{float64(2), math.Inf(-1), map[string]any{"nested": math.NaN()}},
	})

	payload, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("sanitized metadata must be JSON encodable: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("decode sanitized metadata: %v", err)
	}
	if decoded["nan"] != nil || decoded["inf"] != nil {
		t.Fatalf("non-finite floats should become JSON null: %s", payload)
	}
	list, ok := decoded["list"].([]any)
	if !ok || len(list) != 3 || list[1] != nil {
		t.Fatalf("nested non-finite floats should be sanitized: %#v", decoded["list"])
	}
	nested, ok := list[2].(map[string]any)
	if !ok || nested["nested"] != nil {
		t.Fatalf("nested map value should be sanitized: %#v", list[2])
	}
}
