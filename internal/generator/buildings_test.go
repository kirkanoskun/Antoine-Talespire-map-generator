package generator

import (
 "os"
 "reflect"
 "testing"

 "github.com/kirkanoskun/antoine-talespire-map-generator/internal/ir"
 "github.com/kirkanoskun/antoine-talespire-map-generator/internal/prefab"
 "github.com/kirkanoskun/antoine-talespire-map-generator/internal/slab"
 "github.com/kirkanoskun/antoine-talespire-map-generator/internal/spatial"
)

func buildingSetup(t *testing.T) (*Generator,*ir.IR) {
 t.Helper()
 data,err:=os.ReadFile("../../testdata/slabs/precision-v2.txt");if err!=nil { t.Fatal(err) }
 store,err:=prefab.Open("",nil);if err!=nil { t.Fatal(err) }
 if _,err:=store.Add(prefab.Entry{ID:"house",Name:"House",Code:string(data),Width:6});err!=nil { t.Fatal(err) }
 doc,err:=ir.Parse([]byte(`{"map":{"width":20,"length":20,"biome":"temperate_forest"},"zones":[{"id":"land","anchor":{"x":10,"y":10},"relative_size":1}],"buildings":[{"prefab":"house","position":{"x":8,"y":7},"rotation":0}]}`));if err!=nil { t.Fatal(err) }
 g:=newTestGen(t);g.SetPrefabs(store)
 return g,doc
}

func importedInstances(t *testing.T, code string) []slab.Instance {
 t.Helper(); s,err:=slab.DecodeGenerated(code);if err!=nil { t.Fatal(err) }
 var out []slab.Instance
 for _,l:=range s.Layouts { if l.ID[0]==0 && l.ID[1]==1 && l.ID[15]==15 {
  if l.Reserved!=7 { t.Fatal("lost reserved layout bits") }
  out=append(out,l.Instances...)
 } }
 return out
}

func TestBuildingPrecisionRotationAndSlicing(t *testing.T) {
 for _,angle:=range []int{0,90,180,270} { t.Run(string(rune('a'+angle/90)),func(t *testing.T) {
  g,doc:=buildingSetup(t);doc.Buildings[0].Rotation=angle
  res,err:=g.Generate(doc,spatial.Resolve(doc),1);if err!=nil { t.Fatal(err) }
  got:=importedInstances(t,res.Code)
  pairs:=map[int][][2]int{0:{{0,0},{125,125}},90:{{0,125},{125,0}},180:{{125,125},{0,0}},270:{{125,0},{0,125}}}[angle]
  want:=[]slab.Instance{{X:900+pairs[0][0],Y:100,Z:800+pairs[0][1],Rotation:uint8((3+angle/15)%24),Extra:17},{X:900+pairs[1][0],Y:225,Z:800+pairs[1][1],Rotation:uint8((19+angle/15)%24),Extra:5}}
  if !reflect.DeepEqual(got,want) { t.Fatalf("angle %d: got %+v, want %+v",angle,got,want) }
  if !res.Height.BuildingAt(8,7) { t.Fatal("footprint was not reserved") }
  if angle==90 && (!res.Height.BuildingAt(11,12) || res.Height.BuildingAt(12,12)) { t.Fatal("rotated footprint dimensions are wrong") }
  g.SetSliceSize(5)
  sliced,err:=g.Generate(doc,spatial.Resolve(doc),1);if err!=nil { t.Fatal(err) }
  if sliced.Code!=res.Code { t.Fatal("slicing changed whole-map output") }
  count:=0
  for x:=range sliced.Slices { for y,code:=range sliced.Slices[x] {
   parts:=importedInstances(t,code);count+=len(parts)
   for i:=range parts { parts[i].X+=x*500;parts[i].Z+=y*500 }
   if len(parts)>0 && !reflect.DeepEqual(parts,want) { t.Fatal("slicing lost or transformed building parts") }
  } }
  if count!=2 { t.Fatalf("slices contain %d building assets",count) }
 }) }
}

func TestBuildingRejectsUnknownOverlapWaterAndBounds(t *testing.T) {
 for _,kind:=range []string{"unknown","overlap","water","bounds"} { t.Run(kind,func(t *testing.T) {
  g,doc:=buildingSetup(t)
  switch kind {
  case "unknown":doc.Buildings[0].Prefab="missing"
  case "overlap":doc.Buildings=append(doc.Buildings,doc.Buildings[0])
  case "water":doc.Zones[0].ReliefOverride="water"
  case "bounds":doc.Buildings[0].Position.X=19
  }
  if _,err:=g.Generate(doc,spatial.Resolve(doc),1);err==nil { t.Fatal("invalid building placement accepted") }
 }) }
}
