package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/generator"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/prefab"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/server"
)

func TestImportAvailableInPromptAndPersists(t *testing.T) {
	store, err := prefab.Open(filepath.Join(t.TempDir(), "prefabs.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	gen, err := generator.New(repoPath("configs", "biomes.json"), repoPath("configs", "props.json"))
	if err != nil {
		t.Fatal(err)
	}
	h := server.New(gen, server.Options{Prefabs: store, Catalog: testCatalog()}).Handler()
	code, err := os.ReadFile(repoPath("testdata", "slabs", "precision-v2.txt"))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(prefab.Entry{ID: "test_house", Name: "Maison", Code: string(code)})
	rec, _ := post(t, h, "/api/prefabs/import", string(body))
	if rec.Code != 201 {
		t.Fatal(rec.Body.String())
	}
	rec, out := post(t, h, "/api/prompt", `{"description":"une maison"}`)
	prompt, _ := out["prompt"].(string)
	if rec.Code != 200 || !strings.Contains(prompt, "test_house") || strings.Contains(prompt, strings.TrimSpace(string(code))) {
		t.Fatal("prompt must contain metadata but not raw code")
	}
	req := httptest.NewRequest(http.MethodGet, "/api/prefabs", nil)
	list := httptest.NewRecorder()
	h.ServeHTTP(list, req)
	if list.Code != 200 || !strings.Contains(list.Body.String(), "test_house") || strings.Contains(list.Body.String(), `"code"`) {
		t.Fatal("catalogue must expose metadata only")
	}
	rec, _ = post(t, h, "/api/prefabs/import", string(body))
	if rec.Code != 400 {
		t.Fatal("duplicate import accepted")
	}
	rec, _ = post(t, h, "/api/generate", `{"ir":{"map":{"width":12,"length":12,"biome":"temperate_forest"},"zones":[{"id":"a","anchor":{"x":6,"y":6},"relative_size":1}],"buildings":[{"prefab":"test_house","position":{"x":2,"y":2}}]}}`)
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
}
