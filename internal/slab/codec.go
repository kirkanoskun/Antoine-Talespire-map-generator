// Package slab reads and writes the documented TaleSpire v2 packed format.
// Coordinates are integer hundredths of a game unit; Y is the vertical axis.
// Reference: https://github.com/Bouncyrock/DumbSlabStats/blob/master/format.md
package slab

import (
 "bytes"
 "compress/gzip"
 "encoding/base64"
 "encoding/binary"
 "fmt"
 "io"
 "strings"
)

const (
 MaxInputBytes = 1 << 20
 MaxCompressedBytes = 30 * 1024
 MaxDecodedBytes = 4 << 20
 MaxAssets = 100000
 MaxCoordinate = (1 << 18) - 1
)

type Instance struct {
 X, Y, Z int
 Rotation, Extra uint8
}

type Layout struct {
 ID [16]byte
 Reserved uint16
 Instances []Instance
}

type Slab struct { Layouts []Layout }

// Decode accepts a bare code or a single Markdown code fence. It never truncates
// coordinates and rejects unknown versions, creatures and malformed payloads.
func Decode(code string) (*Slab, error) { return decode(code, MaxCompressedBytes) }

// DecodeGenerated permits larger intermediate slabs produced by our generator.
// It has the same decompression and asset-count limits as the public importer.
func DecodeGenerated(code string) (*Slab, error) { return decode(code, MaxInputBytes) }

func decode(code string, compressedLimit int) (*Slab, error) {
 if len(code) > MaxInputBytes { return nil, fmt.Errorf("Slab code exceeds 1 MiB") }
 code = strings.TrimSpace(code)
 if strings.HasPrefix(code, "talespire://") { return nil, fmt.Errorf("this is a Board link; paste a Slab code instead") }
 if strings.HasPrefix(code, "```") {
  if len(code) < 6 || !strings.HasSuffix(code, "```") { return nil, fmt.Errorf("unclosed Slab code fence") }
  code = strings.TrimSpace(code[3:len(code)-3])
  for _, label := range []string{"text\n", "plaintext\n", "slab\n", "base64\n"} {
   code = strings.TrimPrefix(code, label)
  }
 }
 code = strings.Join(strings.Fields(code), "")
 data, err := base64.StdEncoding.Strict().DecodeString(code)
 if err != nil { return nil, fmt.Errorf("invalid Slab base64: %w", err) }
 if len(data) > compressedLimit { return nil, fmt.Errorf("compressed Slab exceeds %d bytes; split it in TaleSpire", compressedLimit) }
 source := bytes.NewReader(data)
 gz, err := gzip.NewReader(source)
 if err != nil { return nil, fmt.Errorf("invalid Slab gzip: %w", err) }
 defer gz.Close()
 gz.Multistream(false)
 raw, err := io.ReadAll(io.LimitReader(gz, MaxDecodedBytes+1))
 if err != nil { return nil, fmt.Errorf("invalid Slab gzip: %w", err) }
 if len(raw) > MaxDecodedBytes { return nil, fmt.Errorf("decompressed Slab exceeds 4 MiB") }
 if source.Len() != 0 { return nil, fmt.Errorf("unexpected data after Slab gzip stream") }
 return parse(raw)
}

func parse(raw []byte) (*Slab, error) {
 if len(raw) < 10 { return nil, fmt.Errorf("Slab header is truncated") }
 le := binary.LittleEndian
 if le.Uint32(raw) != 0xD1CEFACE { return nil, fmt.Errorf("invalid Slab signature") }
 if version := le.Uint16(raw[4:]); version != 2 { return nil, fmt.Errorf("unsupported Slab version %d; only v2 is supported", version) }
 n := int(le.Uint16(raw[6:]))
 if le.Uint16(raw[8:]) != 0 { return nil, fmt.Errorf("creatures are not supported in v2 Slabs") }
 offset := 10 + 20*n
 if len(raw) < offset { return nil, fmt.Errorf("Slab layout table is truncated") }
 out := &Slab{Layouts: make([]Layout, n)}
 total := 0
 for i := range out.Layouts {
  at := 10 + i*20
  count := int(le.Uint16(raw[at+16:]))
  total += count
  if total > MaxAssets { return nil, fmt.Errorf("Slab exceeds %d assets", MaxAssets) }
  copy(out.Layouts[i].ID[:], raw[at:at+16])
  out.Layouts[i].Reserved = le.Uint16(raw[at+18:])
  out.Layouts[i].Instances = make([]Instance, count)
 }
 end := offset + total*8
 // talescoder v1.0.5 emits a final zero uint16; tolerate exactly that padding.
 if len(raw) != end && !(len(raw) == end+2 && le.Uint16(raw[end:]) == 0) {
  return nil, fmt.Errorf("Slab asset data length does not match layout counts")
 }
 for i := range out.Layouts {
  for j := range out.Layouts[i].Instances {
   packed := le.Uint64(raw[offset:]); offset += 8
   p := Instance{X:int(packed & MaxCoordinate), Y:int((packed>>18)&MaxCoordinate), Z:int((packed>>36)&MaxCoordinate), Rotation:uint8((packed>>54)&31), Extra:uint8(packed>>59)}
   if p.Rotation >= 24 { return nil, fmt.Errorf("invalid Slab rotation %d", p.Rotation) }
   out.Layouts[i].Instances[j] = p
  }
 }
 return out, nil
}

func Encode(s *Slab) (string, error) {
 if s == nil || len(s.Layouts) > 65535 { return "", fmt.Errorf("invalid Slab layout count") }
 var raw bytes.Buffer
 write := func(v any) { _ = binary.Write(&raw, binary.LittleEndian, v) }
 write(uint32(0xD1CEFACE)); write(uint16(2)); write(uint16(len(s.Layouts))); write(uint16(0))
 total := 0
 for _, l := range s.Layouts {
  total += len(l.Instances)
  if len(l.Instances) > 65535 || total > MaxAssets { return "", fmt.Errorf("too many Slab assets") }
  raw.Write(l.ID[:]); write(uint16(len(l.Instances))); write(l.Reserved)
 }
 for _, l := range s.Layouts {
  for _, p := range l.Instances {
   if p.X < 0 || p.Y < 0 || p.Z < 0 || p.X > MaxCoordinate || p.Y > MaxCoordinate || p.Z > MaxCoordinate || p.Rotation >= 24 || p.Extra > 31 {
    return "", fmt.Errorf("Slab position or rotation out of bounds")
   }
   write(uint64(p.X) | uint64(p.Y)<<18 | uint64(p.Z)<<36 | uint64(p.Rotation)<<54 | uint64(p.Extra)<<59)
  }
 }
 if raw.Len() > MaxDecodedBytes { return "", fmt.Errorf("Slab exceeds decoded size limit") }
 var compressed bytes.Buffer
 gz := gzip.NewWriter(&compressed)
 if _, err := gz.Write(raw.Bytes()); err != nil { return "", err }
 if err := gz.Close(); err != nil { return "", err }
 return base64.StdEncoding.EncodeToString(compressed.Bytes()), nil
}

func (s *Slab) Count() int {
 n := 0
 for _, l := range s.Layouts { n += len(l.Instances) }
 return n
}
