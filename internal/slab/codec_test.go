package slab

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/binary"
	"os"
	"reflect"
	"strings"
	"testing"
)

func fixture(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("../../testdata/slabs/precision-v2.txt")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestOfficialPackedFixtureAndRoundTrip(t *testing.T) {
	code := fixture(t)
	for _, wrapped := range []string{code, "```" + code + "```", "```text\n" + code + "```", " \n" + code + " "} {
		got, err := Decode(wrapped)
		if err != nil {
			t.Fatal(err)
		}
		want := []Instance{{X: 125, Y: 50, Z: 375, Rotation: 3, Extra: 17}, {X: 250, Y: 175, Z: 500, Rotation: 19, Extra: 5}}
		if !reflect.DeepEqual(got.Layouts[0].Instances, want) || got.Layouts[0].Reserved != 7 {
			t.Fatalf("precision lost: %+v", got)
		}
		out, err := Encode(got)
		if err != nil {
			t.Fatal(err)
		}
		again, err := Decode(out)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, again) {
			t.Fatal("round-trip changed coordinates, rotation, IDs or reserved bits")
		}
	}
}

func zipped(t *testing.T, raw []byte) string {
	t.Helper()
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	if _, err := w.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(b.Bytes())
}

func TestRejectsMalformedAndOversized(t *testing.T) {
	data, _ := base64.StdEncoding.DecodeString(strings.TrimSpace(fixture(t)))
	raw := make([]byte, 30+16)
	binary.LittleEndian.PutUint32(raw, 0xD1CEFACE)
	binary.LittleEndian.PutUint16(raw[4:], 2)
	binary.LittleEndian.PutUint16(raw[6:], 1)
	binary.LittleEndian.PutUint16(raw[26:], 2)
	cases := []string{"", "not base64!", "talespire://board/123", "```oops", strings.Repeat("A", MaxInputBytes+1), base64.StdEncoding.EncodeToString([]byte("not gzip")), base64.StdEncoding.EncodeToString(data[:len(data)-3]), base64.StdEncoding.EncodeToString(append(data, 1)), zipped(t, []byte("short")), zipped(t, make([]byte, MaxDecodedBytes+1))}
	for _, index := range []int{0, 4, 8, 26, 29} {
		b := append([]byte(nil), raw...)
		b[index] = 255
		// Reserved fields are preserved; do not use index 29 for a malformed test.
		if index == 29 {
			continue
		}
		cases = append(cases, zipped(t, b))
	}
	invalidRot := append([]byte(nil), raw...)
	binary.LittleEndian.PutUint64(invalidRot[30:], uint64(31)<<54)
	cases = append(cases, zipped(t, invalidRot))
	cases = append(cases, zipped(t, raw[:len(raw)-1]), zipped(t, append(raw, 1, 0, 0)))
	for i, code := range cases {
		if _, err := Decode(code); err == nil {
			t.Errorf("case %d accepted invalid Slab", i)
		}
	}
}

func TestEncodeCoordinateOverflow(t *testing.T) {
	_, err := Encode(&Slab{Layouts: []Layout{{Instances: []Instance{{X: MaxCoordinate + 1}}}}})
	if err == nil {
		t.Fatal("coordinate overflow accepted")
	}
}
