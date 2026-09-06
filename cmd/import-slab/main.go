// Command import-slab validates a pasted code and adds it to a portable catalogue.
package main

import (
 "encoding/json"
 "flag"
 "fmt"
 "io"
 "log"
 "os"

 "github.com/kirkanoskun/antoine-talespire-map-generator/internal/prefab"
 "github.com/kirkanoskun/antoine-talespire-map-generator/internal/slab"
)

func main() {
 input := flag.String("input", "-", "Slab text file, or - for stdin")
 catalog := flag.String("catalog", "configs/prefabs.json", "portable prefab catalogue to update")
 id := flag.String("id", "", "unique snake_case id (required)")
 name := flag.String("name", "", "building name (defaults to id)")
 author := flag.String("author", "", "creator, if known")
 source := flag.String("source", "", "source URL, if known")
 description := flag.String("description", "", "short description for the model")
 width := flag.Int("width", 0, "optional larger reserved footprint in tiles")
 length := flag.Int("length", 0, "optional larger reserved footprint in tiles")
 flag.Parse()
 var r io.Reader = os.Stdin
 if *input != "-" {
  f, err := os.Open(*input); if err != nil { log.Fatal(err) }; defer f.Close(); r = f
 }
 code, err := io.ReadAll(io.LimitReader(r, slab.MaxInputBytes+1))
 if err != nil { log.Fatal(err) }
 if *name == "" { *name = *id }
 store, err := prefab.Open(*catalog, nil)
 if err != nil { log.Fatal(err) }
 info, err := store.Add(prefab.Entry{ID:*id, Name:*name, Code:string(code), Author:*author, SourceURL:*source, Description:*description, Width:*width, Length:*length})
 if err != nil { log.Fatal(err) }
 result, _ := json.MarshalIndent(info, "", "  ")
 fmt.Println(string(result))
}
