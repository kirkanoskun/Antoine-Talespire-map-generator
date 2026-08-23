package chimera

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/binary"
	"strings"
	"testing"
)

// TestPositionRoundTrip: encode∘decode is the identity for the packed layout.
func TestPositionRoundTrip(t *testing.T) {
	cases := []Placement{
		{RawX: 0, RawY: 0, RawZ: 0, RotStep: 0},
		{RawX: 2300, RawY: 3000, RawZ: 300, RotStep: 6},     // tile (23, 30), h6, 90°
		{RawX: 100, RawY: 2500, RawZ: 800, RotStep: 23},     // large-ish Y, 345°
		{RawX: 65500, RawY: 65500, RawZ: 65500, RotStep: 0}, // near field max
	}
	for _, want := range cases {
		got := decodePosition(encodePosition(want))
		if got.RawX != want.RawX || got.RawY != want.RawY || got.RawZ != want.RawZ || got.RotStep != want.RotStep {
			t.Errorf("round-trip mismatch:\n want raw x=%d y=%d z=%d rot=%d\n got  raw x=%d y=%d z=%d rot=%d",
				want.RawX, want.RawY, want.RawZ, want.RotStep, got.RawX, got.RawY, got.RawZ, got.RotStep)
		}
	}
}

func TestTileConversion(t *testing.T) {
	p := decodePosition(encodePosition(Placement{RawX: 2300, RawY: 3000, RawZ: 300, RotStep: 8}))
	if p.TileX != 23 || p.TileY != 30 || p.Height != 6 || p.Degrees != 120 {
		t.Errorf("tile conversion wrong: x=%v y=%v h=%v deg=%d (want 23,30,6,120)", p.TileX, p.TileY, p.Height, p.Degrees)
	}
}

// talescoderMisread mimics talescoder v1.0.5: it reads the position as four
// byte-aligned uint16s and runs its buggy DecodeY on the third one.
func talescoderMisreadY(pos []byte) (y uint16, rotation uint16) {
	centerY := binary.LittleEndian.Uint16(pos[4:6]) // talescoder's "centerY"
	rotation = binary.LittleEndian.Uint16(pos[6:8])
	// axisadapter.DecodeY, verbatim.
	result1 := centerY / 1600
	remain1 := centerY % 1600
	result2 := remain1 / 64
	return result1 + 41*result2, rotation
}

// TestReproducesTalescoderYCorruption encodes a placement at a realistic
// sub-tile position (as community buildings' rotated/offset pieces are) and
// shows (a) chimera decodes Y correctly, and (b) talescoder's misaligned read
// mangles it. talescoder is only accidentally right when rawY is an exact
// multiple of 100 (grid-snapped) — which is why the monastère's floor decoded
// fine while its decorations ran to impossible Y values.
func TestReproducesTalescoderYCorruption(t *testing.T) {
	// Tile ~ (5, 25.37) — a decoration not snapped to the grid. Height 6, 90°.
	src := Placement{RawX: 500, RawY: 2537, RawZ: 300, RotStep: 6}
	pos := encodePosition(src)

	got := decodePosition(pos)
	if got.RawY != 2537 || abs(got.TileY-25.37) > 1e-9 || got.Degrees != 90 {
		t.Fatalf("chimera should decode Y=25.37, 90°, got Y=%v deg=%d", got.TileY, got.Degrees)
	}

	tsY, tsRot := talescoderMisreadY(pos)
	if uint32(tsY) == src.RawY/100 || tsY < 100 {
		t.Errorf("expected talescoder Y to blow up, got %d", tsY)
	}
	// Rotation still reads as a clean multiple of 64 (rotStep*64) because this
	// Y stays under 4096 units — matching the monastère, where rotations were
	// multiples of 64 yet Y was garbage.
	if tsRot != uint16(src.RotStep)*64 {
		t.Errorf("expected talescoder rotation = %d (step*64), got %d", src.RotStep*64, tsRot)
	}
	t.Logf("chimera Y=25.37 (correct); talescoder Y=%d, rotation=%d (corrupted)", tsY, tsRot)
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

// buildSlab assembles a minimal but structurally real gzipped+base64 slab.
func buildSlab(t *testing.T, version int16, assets []Asset) string {
	t.Helper()
	var buf bytes.Buffer
	buf.Write([]byte{0xff, 0xff, 0xff, 0xff}) // magic (ignored on decode)
	putInt16(&buf, version)
	putInt16(&buf, int16(len(assets)))
	for _, a := range assets {
		id, err := base64.StdEncoding.DecodeString(a.IDBase64)
		if err != nil || len(id) != 18 {
			t.Fatalf("asset id must be 18 bytes base64, got %d (%v)", len(id), err)
		}
		buf.Write(id)
		putInt16(&buf, int16(len(a.Placements)))
	}
	putInt16(&buf, 0) // two-byte separator
	for _, a := range assets {
		for _, p := range a.Placements {
			buf.Write(encodePosition(p))
		}
	}
	var gzbuf bytes.Buffer
	gw := gzip.NewWriter(&gzbuf)
	if _, err := gw.Write(buf.Bytes()); err != nil {
		t.Fatal(err)
	}
	gw.Close()
	return base64.StdEncoding.EncodeToString(gzbuf.Bytes())
}

func putInt16(buf *bytes.Buffer, v int16) {
	b := make([]byte, 2)
	binary.LittleEndian.PutUint16(b, uint16(v))
	buf.Write(b)
}

func TestDecodeIgnoresWhitespace(t *testing.T) {
	id := "AAAQosMB+5SfRIxHmT7aPnEm"
	code := buildSlab(t, 2, []Asset{{IDBase64: id, Placements: []Placement{{RawX: 0, RawY: 0, RawZ: 0, RotStep: 0}}}})
	// Simulate a wrapped paste.
	wrapped := code[:20] + "\n" + code[20:40] + "  \r\n" + code[40:]
	if _, err := Decode(wrapped); err != nil {
		t.Errorf("Decode should tolerate whitespace, got: %v", err)
	}
}

func TestDecodeReportsTruncation(t *testing.T) {
	id := "AAAQosMB+5SfRIxHmT7aPnEm"
	code := buildSlab(t, 2, []Asset{{IDBase64: id, Placements: []Placement{{RawX: 0, RawY: 0, RawZ: 0, RotStep: 0}}}})
	// Drop one character so the length is 1 (mod 4) — impossible for base64.
	_, err := Decode(code[:len(code)-1])
	if err == nil || !strings.Contains(err.Error(), "truncated") {
		t.Errorf("expected a truncation error, got: %v", err)
	}
}

// TestFullSlabDecode exercises the whole pipeline (base64 → gunzip → parse →
// unpack) on a slab we built, including a large Y that talescoder would break.
func TestFullSlabDecode(t *testing.T) {
	id := "AAAQosMB+5SfRIxHmT7aPnEm" // 18-byte id from the monastère dump
	in := []Asset{{
		IDBase64: id,
		Placements: []Placement{
			{RawX: 0, RawY: 0, RawZ: 300, RotStep: 0},
			{RawX: 2200, RawY: 3000, RawZ: 300, RotStep: 6}, // tile (22,30)
		},
	}}
	code := buildSlab(t, 2, in)

	slab, err := Decode(code)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if slab.Version != 2 || len(slab.Assets) != 1 {
		t.Fatalf("header wrong: version=%d assets=%d", slab.Version, len(slab.Assets))
	}
	a := slab.Assets[0]
	if a.IDBase64 != id {
		t.Errorf("asset id = %q, want %q", a.IDBase64, id)
	}
	if len(a.Placements) != 2 {
		t.Fatalf("got %d placements, want 2", len(a.Placements))
	}
	p := a.Placements[1]
	if p.TileX != 22 || p.TileY != 30 || p.Height != 6 || p.Degrees != 90 {
		t.Errorf("placement decoded wrong: x=%v y=%v h=%v deg=%d (want 22,30,6,90)", p.TileX, p.TileY, p.Height, p.Degrees)
	}
	// Every Y must be sane (small), never the 0..1009 blow-up.
	for _, pl := range a.Placements {
		if pl.TileY > 1000 {
			t.Errorf("Y blew up: %v", pl.TileY)
		}
	}
}
