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
		"typed_maps": []map[string]any{
			{"stream_index": 0, "avg_frame_rate": math.Inf(1)},
			{"stream_index": 1, "duration": math.NaN()},
		},
		"float_map": map[string]float64{"good": 4.2, "bad": math.Inf(-1)},
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
	typedMaps, ok := decoded["typed_maps"].([]any)
	if !ok || len(typedMaps) != 2 {
		t.Fatalf("typed map slice should be converted to JSON-safe list: %#v", decoded["typed_maps"])
	}
	first, ok := typedMaps[0].(map[string]any)
	if !ok || first["avg_frame_rate"] != nil {
		t.Fatalf("typed map slice should sanitize nested non-finite floats: %#v", typedMaps[0])
	}
	floatMap, ok := decoded["float_map"].(map[string]any)
	if !ok || floatMap["bad"] != nil || floatMap["good"] == nil {
		t.Fatalf("typed float map should be JSON-safe: %#v", decoded["float_map"])
	}
}
