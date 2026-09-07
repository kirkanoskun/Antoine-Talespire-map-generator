package ir

import (
	"encoding/json"
	"testing"
)

func TestParseRejectsTrailingData(t *testing.T) {
	valid := `{"map":{"width":4,"length":4,"biome":"temperate_forest"},"zones":[{"id":"a","anchor":{"x":2,"y":2},"relative_size":1}]}`
	for _, suffix := range []string{" {}", " null", " garbage"} {
		if _, err := Parse([]byte(valid + suffix)); err == nil {
			t.Errorf("accepted trailing data %q", suffix)
		}
	}
	if _, err := Parse([]byte(valid + " \n")); err != nil {
		t.Fatal(err)
	}
}

func TestPositionRejectsUnknownFieldsAndResets(t *testing.T) {
	var p Position
	if err := json.Unmarshal([]byte(`{"x":1,"y":2,"z":3}`), &p); err == nil {
		t.Fatal("accepted unknown position field")
	}
	if err := json.Unmarshal([]byte(`"scattered"`), &p); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{"x":1,"y":2}`), &p); err != nil {
		t.Fatal(err)
	}
	if p.Scattered || p.X != 1 || p.Y != 2 {
		t.Fatalf("stale position: %+v", p)
	}
}

func TestBuildingPlacementValidation(t *testing.T) {
	for _, b := range []string{`{"prefab":"house"}`, `{"position":{"x":1,"y":1}}`, `{"prefab":"house","position":{"x":1,"y":1},"rotation":45}`} {
		raw := `{"map":{"width":10,"length":10,"biome":"temperate_forest"},"zones":[{"id":"a","anchor":{"x":5,"y":5},"relative_size":1}],"buildings":[` + b + `]}`
		if _, err := Parse([]byte(raw)); err == nil {
			t.Fatal("invalid building accepted")
		}
	}
}
