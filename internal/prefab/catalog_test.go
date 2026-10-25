package prefab

import (
	"os"
	"path/filepath"
	"testing"
)

func sample(t *testing.T) Entry {
	t.Helper()
	data, err := os.ReadFile("../../testdata/slabs/precision-v2.txt")
	if err != nil {
		t.Fatal(err)
	}
	return Entry{ID: "test_house", Name: "Test house", Code: string(data)}
}

func TestPersistentImportAndIsolation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prefabs.json")
	store, err := Open(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	entry := sample(t)
	info, err := store.Add(entry)
	if err != nil {
		t.Fatal(err)
	}
	if info.Width != 4 || info.Length != 4 || info.AssetCount != 2 {
		t.Fatalf("wrong metadata %+v", info)
	}
	if _, err := store.Add(entry); err == nil {
		t.Fatal("duplicate id accepted")
	}
	entry.ID = "duplicate_code"
	if _, err := store.Add(entry); err == nil {
		t.Fatal("duplicate code accepted")
	}
	loaded, err := Open(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, ok := loaded.Get("test_house")
	if !ok {
		t.Fatal("lost import after reopening")
	}
	if b.Slab.Layouts[0].Instances[0].X != 0 || b.Slab.Layouts[0].Instances[1].Y != 125 {
		t.Fatal("relative coordinates lost")
	}
	b.Slab.Layouts[0].Instances[0].X = 999
	again, _ := loaded.Get("test_house")
	if again.Slab.Layouts[0].Instances[0].X != 0 {
		t.Fatal("caller mutated stored prefab")
	}
}

func TestImportDoesNotSaveInvalidCode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prefabs.json")
	store, err := Open(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	e := sample(t)
	e.Code = "bad"
	if _, err := store.Add(e); err == nil {
		t.Fatal("invalid slab saved")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("invalid import touched file")
	}
	if len(store.List()) != 0 {
		t.Fatal("invalid import touched memory")
	}
	e = sample(t)
	e.Width = 1
	if _, err := store.Add(e); err == nil {
		t.Fatal("undersized footprint accepted")
	}
}
