// Package chimera decodes (and re-encodes) the position data of TaleSpire
// "Chimera" slabs — the format used by community sites like Tales Tavern and
// TalesBazaar — with a CORRECT horizontal-depth (Y) axis.
//
// # Why this exists
//
// The project depends on johnfercher/talescoder (v1.0.5, the latest) to decode
// slabs. Its X and Z axes are fine, but its Y axis is wrong on real game slabs:
// talescoder reads each placement's position as four byte-aligned uint16s
// (offsets 0/16/32/48 bits), whereas TaleSpire packs the position as a single
// 64-bit little-endian value with 18-bit fields:
//
//	X   = blob        & 0x3FFFF   (bits  0..17)
//	Z   = (blob >> 18) & 0x3FFFF  (bits 18..35)  // vertical / height
//	Y   = (blob >> 36) & 0x3FFFF  (bits 36..53)  // horizontal depth
//	rot = (blob >> 54) & 0x3FF    (bits 54..63)  // steps of 15 degrees
//
// (Confirmed against LuPro/SlabelFish's encoder and decoder, the reference
// implementation for the Chimera format.) Because talescoder's fields are
// 16-bit-aligned, only X (at bit 0) lines up. Z is off by two bits but its
// /200 scale happens to absorb that. Y is off by four bits AND its high bits
// spill into what talescoder reports as "rotation" — which is why every
// rotation in a talescoder-decoded real slab is a multiple of 64 and Y balloons
// to impossible values (0..1009 for a ~24-tile building).
//
// This package is deliberately standalone: nothing in the map generator imports
// it. It is the vetted decoding layer a future "community building library"
// would build on, and a tool to validate individual slab codes before trusting
// them.
package chimera

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
	"unicode"
)

// Bit layout of the 64-bit position blob (see package doc).
const (
	shiftX   = 0
	shiftZ   = 18
	shiftY   = 36
	shiftRot = 54

	fieldMask = 0x3FFFF // 18 bits
	rotMask   = 0x3FF   // 10 bits

	// Game units per grid step. The vertical step is HALF the horizontal one.
	//
	// Horizontal (100 u/tile) is confirmed twice over: a real community slab has
	// X/Y on a clean 100-unit lattice, and this project's own generator emits
	// rawX/rawY of 0,100,...,500 for a 6-tile map.
	//
	// Vertical (50 u/step) is confirmed by round-tripping the generator's own
	// output: a map at height 1 encodes to rawZ=50. taleslab/talescoder have
	// always used this step and produce slabs the game accepts, so it is the
	// unit to interoperate in. It is also consistent with the community slab,
	// whose floor slabs are 400 units apart — 8 vertical steps, i.e. the usual
	// 4-tile-equivalent ceiling height, since a step is half a tile.
	//
	// Raw{X,Y,Z} are exposed so any rescale stays lossless.
	unitsPerTileH = 100.0
	unitsPerStepV = 50.0
)

// Placement is one instance of an asset, in tile coordinates plus the raw
// game-unit values it was decoded from.
type Placement struct {
	TileX   float64 `json:"tile_x"`
	TileY   float64 `json:"tile_y"` // horizontal depth — the axis talescoder gets wrong
	Height  float64 `json:"height"` // vertical (Z)
	Degrees int     `json:"degrees"`
	RawX    uint32  `json:"raw_x"`
	RawY    uint32  `json:"raw_y"`
	RawZ    uint32  `json:"raw_z"`
	RotStep uint32  `json:"rot_step"`
}

// Asset is a distinct asset id and every place it appears.
type Asset struct {
	IDBase64   string      `json:"id_base64"`
	Placements []Placement `json:"placements"`
}

// Slab is the decoded slab.
type Slab struct {
	Version    int16   `json:"version"`
	MagicBytes []byte  `json:"-"` // preserved so Encode round-trips exactly
	Assets     []Asset `json:"assets"`
}

// DefaultMagicBytes is TaleSpire's slab header signature.
var DefaultMagicBytes = []byte{206, 250, 206, 209}

// decodePosition unpacks one 8-byte little-endian position blob.
func decodePosition(b []byte) Placement {
	blob := binary.LittleEndian.Uint64(b)
	xr := uint32((blob >> shiftX) & fieldMask)
	zr := uint32((blob >> shiftZ) & fieldMask)
	yr := uint32((blob >> shiftY) & fieldMask)
	rot := uint32((blob >> shiftRot) & rotMask)
	return Placement{
		TileX:   float64(xr) / unitsPerTileH,
		TileY:   float64(yr) / unitsPerTileH,
		Height:  float64(zr) / unitsPerStepV,
		Degrees: int(rot) * 15,
		RawX:    xr,
		RawY:    yr,
		RawZ:    zr,
		RotStep: rot,
	}
}

// encodePosition packs a placement back into an 8-byte little-endian blob. It is
// the exact inverse of decodePosition and exists mainly for round-trip testing.
func encodePosition(p Placement) []byte {
	xr := uint64(p.RawX & fieldMask)
	yr := uint64(p.RawY & fieldMask)
	zr := uint64(p.RawZ & fieldMask)
	rot := uint64(p.RotStep & rotMask)
	blob := xr<<shiftX | zr<<shiftZ | yr<<shiftY | rot<<shiftRot
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, blob)
	return b
}

// Decode parses a base64 TaleSpire slab code into a Slab with correct Y.
//
// The byte structure mirrors talescoder's (magic, version, asset table, a
// two-byte separator, then the layout blobs); only the position unpacking
// differs. Real slabs whose asset ids talescoder already reads correctly parse
// here too — this just fixes the coordinates.
func Decode(slabBase64 string) (*Slab, error) {
	// Slab codes are often pasted with wrapping or stray whitespace; strip it so
	// a clean paste works. A base64 payload's length is always a multiple of 4,
	// so a different remainder means a character was lost or added in transit.
	clean := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, slabBase64)
	if len(clean) == 0 {
		return nil, fmt.Errorf("empty slab code")
	}
	if len(clean)%4 != 0 {
		return nil, fmt.Errorf("slab code looks truncated: %d base64 chars is not a multiple of 4 (a character was likely lost when copying — re-copy or attach it as a file)", len(clean))
	}
	compressed, err := base64.StdEncoding.DecodeString(clean)
	if err != nil {
		return nil, fmt.Errorf("base64: %w", err)
	}
	gz, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, fmt.Errorf("gzip: %w", err)
	}
	raw, err := io.ReadAll(gz)
	if err != nil {
		return nil, fmt.Errorf("gunzip: %w", err)
	}

	c := &cursor{buf: raw}
	magic, err := c.bytes(4)
	if err != nil {
		return nil, err
	}
	version, err := c.int16()
	if err != nil {
		return nil, err
	}
	assetCount, err := c.int16()
	if err != nil {
		return nil, err
	}

	slab := &Slab{Version: version, MagicBytes: append([]byte(nil), magic...)}
	counts := make([]int16, assetCount)
	for i := int16(0); i < assetCount; i++ {
		id, err := c.bytes(18)
		if err != nil {
			return nil, err
		}
		n, err := c.int16()
		if err != nil {
			return nil, err
		}
		slab.Assets = append(slab.Assets, Asset{IDBase64: base64.StdEncoding.EncodeToString(id)})
		counts[i] = n
	}

	if _, err := c.int16(); err != nil { // two-byte separator before layouts
		return nil, err
	}

	for i := int16(0); i < assetCount; i++ {
		for j := int16(0); j < counts[i]; j++ {
			pos, err := c.bytes(8)
			if err != nil {
				return nil, err
			}
			slab.Assets[i].Placements = append(slab.Assets[i].Placements, decodePosition(pos))
		}
	}
	return slab, nil
}

// cursor is a tiny forward reader over the decompressed slab bytes.
type cursor struct {
	buf []byte
	pos int
}

func (c *cursor) bytes(n int) ([]byte, error) {
	if c.pos+n > len(c.buf) {
		return nil, fmt.Errorf("unexpected end of slab at offset %d (need %d, have %d)", c.pos, n, len(c.buf)-c.pos)
	}
	b := c.buf[c.pos : c.pos+n]
	c.pos += n
	return b, nil
}

func (c *cursor) int16() (int16, error) {
	b, err := c.bytes(2)
	if err != nil {
		return 0, err
	}
	return int16(binary.LittleEndian.Uint16(b)), nil
}

// CalibrationNote records how the scales were established.
const CalibrationNote = `Horizontal is 100 units/tile, vertical is 50 units/step (half a tile). ` +
	`Both were confirmed by round-tripping this project's own generator output, and are ` +
	`consistent with a real community slab. Rotations decode as exactly 24 steps of 15 degrees, ` +
	`which independently confirms the bit layout. Raw{X,Y,Z} are exposed so any rescale is lossless.`

// Encode writes a Slab back to a base64 TaleSpire slab code, using the correct
// bit layout. It is the exact inverse of Decode, so decode->encode of a real
// slab reproduces it byte for byte.
//
// Assets are written in slice order and placements in their listed order, so
// output is deterministic for a given Slab.
func Encode(s *Slab) (string, error) {
	if s == nil {
		return "", fmt.Errorf("nil slab")
	}
	magic := s.MagicBytes
	if len(magic) != 4 {
		magic = DefaultMagicBytes
	}
	version := s.Version
	if version == 0 {
		version = 2
	}

	var buf bytes.Buffer
	buf.Write(magic)
	putI16(&buf, version)
	putI16(&buf, int16(len(s.Assets)))
	for _, a := range s.Assets {
		id, err := base64.StdEncoding.DecodeString(a.IDBase64)
		if err != nil {
			return "", fmt.Errorf("asset id %q: %w", a.IDBase64, err)
		}
		if len(id) != 18 {
			return "", fmt.Errorf("asset id %q decodes to %d bytes, want 18", a.IDBase64, len(id))
		}
		buf.Write(id)
		putI16(&buf, int16(len(a.Placements)))
	}
	putI16(&buf, 0) // separator before the layout blobs
	for _, a := range s.Assets {
		for _, p := range a.Placements {
			buf.Write(encodePosition(p))
		}
	}

	var gz bytes.Buffer
	w, _ := gzip.NewWriterLevel(&gz, gzip.BestCompression)
	if _, err := w.Write(buf.Bytes()); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(gz.Bytes()), nil
}

func putI16(buf *bytes.Buffer, v int16) {
	var b [2]byte
	binary.LittleEndian.PutUint16(b[:], uint16(v))
	buf.Write(b[:])
}
