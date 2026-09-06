package server

import (
 "net/http"

 "github.com/kirkanoskun/antoine-talespire-map-generator/internal/prefab"
)

func (s *Server) handlePrefabs(w http.ResponseWriter, r *http.Request) {
 if r.Method != http.MethodGet { writeError(w, http.StatusMethodNotAllowed, "GET required"); return }
 writeJSON(w, http.StatusOK, s.prefabs.List())
}

func (s *Server) handleImportPrefab(w http.ResponseWriter, r *http.Request) {
 var entry prefab.Entry
 if !decodeBody(w, r, &entry) { return }
 if s.prefabs == nil { writeError(w, http.StatusServiceUnavailable, "prefab import is not configured"); return }
 info, err := s.prefabs.Add(entry)
 if err != nil { writeError(w, http.StatusBadRequest, err.Error()); return }
 writeJSON(w, http.StatusCreated, info)
}
