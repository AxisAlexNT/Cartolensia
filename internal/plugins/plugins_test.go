package plugins

import "testing"

func TestSortOrdersDependencies(t *testing.T) {
	got, err := Sort([]Manifest{
		{ID: "b", Name: "B", Version: "1.0.0", DependsOn: []string{"a"}},
		{ID: "a", Name: "A", Version: "1.0.0"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].ID != "a" || got[1].ID != "b" {
		t.Fatalf("unexpected order %#v", got)
	}
}

func TestSortRejectsMissingDependency(t *testing.T) {
	_, err := Sort([]Manifest{{ID: "b", Name: "B", Version: "1.0.0", DependsOn: []string{"a"}}})
	if err == nil {
		t.Fatal("expected missing dependency error")
	}
}

func TestSortRejectsCycle(t *testing.T) {
	_, err := Sort([]Manifest{
		{ID: "a", Name: "A", Version: "1.0.0", DependsOn: []string{"b"}},
		{ID: "b", Name: "B", Version: "1.0.0", DependsOn: []string{"a"}},
	})
	if err == nil {
		t.Fatal("expected cycle error")
	}
}

func TestSortRejectsDuplicateID(t *testing.T) {
	_, err := Sort([]Manifest{
		{ID: "a", Name: "A", Version: "1.0.0"},
		{ID: "a", Name: "Duplicate", Version: "1.0.0"},
	})
	if err == nil {
		t.Fatal("expected duplicate plugin id error")
	}
}

func TestSidecarHTTPValidationAndDefaults(t *testing.T) {
	_, err := Sort([]Manifest{{ID: "sidecar", Name: "Sidecar", Version: "1.0.0", Runtime: "sidecar_http"}})
	if err == nil {
		t.Fatal("expected missing sidecar base url error")
	}
	got, err := Sort([]Manifest{{
		ID:      "sidecar",
		Name:    "Sidecar",
		Version: "1.0.0",
		Runtime: "sidecar_http",
		SidecarHTTP: &SidecarHTTP{
			BaseURL: "http://127.0.0.1:19090",
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Status != "loaded" || got[0].SidecarHTTP.HealthPath != "/health" {
		t.Fatalf("expected sidecar defaults, got %#v", got[0])
	}
}

func TestSortDefaultsRuntime(t *testing.T) {
	got, err := Sort([]Manifest{{ID: "a", Name: "A", Version: "1.0.0"}})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Runtime != "builtin" || got[0].Status != "loaded" {
		t.Fatalf("expected normalized manifest, got %#v", got[0])
	}
}

func TestBuiltInsContainRequestedPlugins(t *testing.T) {
	got, err := Sort(BuiltIns())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"albums": false, "mapview": false, "gpstracks": false, "transcoding": false, "ai-base": false, "ai-classification": false}
	for _, manifest := range got {
		if _, ok := want[manifest.ID]; ok {
			want[manifest.ID] = true
		}
	}
	for id, seen := range want {
		if !seen {
			t.Fatalf("missing built-in plugin %s", id)
		}
	}
}
