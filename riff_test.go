// Copyright 2026 <chaishushan{AT}gmail.com>. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webp

import (
	"bytes"
	"encoding/binary"
	"os"
	"testing"
)

// buildRIFF assembles a RIFF/WEBP byte stream from raw chunk payloads,
// applying the RIFF pad byte after odd-size chunks.
func buildRIFF(t *testing.T, chunks ...[2][]byte) []byte {
	t.Helper()
	var body bytes.Buffer
	body.WriteString("WEBP")
	for _, c := range chunks {
		fourcc, payload := c[0], c[1]
		if len(fourcc) != 4 {
			t.Fatalf("bad fourcc %q", fourcc)
		}
		body.Write(fourcc)
		var sz [4]byte
		binary.LittleEndian.PutUint32(sz[:], uint32(len(payload)))
		body.Write(sz[:])
		body.Write(payload)
		if len(payload)%2 != 0 {
			body.WriteByte(0)
		}
	}
	var out bytes.Buffer
	out.WriteString("RIFF")
	var sz [4]byte
	binary.LittleEndian.PutUint32(sz[:], uint32(body.Len()))
	out.Write(sz[:])
	out.Write(body.Bytes())
	return out.Bytes()
}

// tinyLosslessWebP returns a real 1x1 lossless WebP produced by the encoder,
// used as a known-good container in parser tests.
func tinyLosslessWebP(t *testing.T) []byte {
	t.Helper()
	data, err := EncodeLosslessRGBA(newUniformRGBA(1, 1, 0xff))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestParseWebPContainer_Valid(t *testing.T) {
	data := tinyLosslessWebP(t)
	chunks, err := parseWebPContainer(data)
	if err != nil {
		t.Fatalf("valid container rejected: %v", err)
	}
	if len(chunks) == 0 || (chunks[0].FourCC != "VP8L" && chunks[0].FourCC != "VP8 ") {
		t.Fatalf("unexpected first chunk: %+v", chunks)
	}
}

func TestParseWebPContainer_AllFixtures(t *testing.T) {
	// Every shipped fixture must pass container validation.
	files, err := os.ReadDir(testdataDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if len(f.Name()) < 5 || f.Name()[len(f.Name())-5:] != ".webp" {
			continue
		}
		data, err := os.ReadFile(testdataDir + f.Name())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := parseWebPContainer(data); err != nil {
			t.Fatalf("%s: %v", f.Name(), err)
		}
	}
}

func TestParseWebPContainer_BadMagic(t *testing.T) {
	good := tinyLosslessWebP(t)

	for _, tc := range []struct {
		name string
		mod  func([]byte)
	}{
		{"riff", func(d []byte) { copy(d[0:4], "RIFX") }},
		{"webp", func(d []byte) { copy(d[8:12], "WAVE") }},
		{"first-chunk", func(d []byte) { copy(d[12:16], "JUNK") }},
	} {
		data := append([]byte(nil), good...)
		tc.mod(data)
		if _, err := parseWebPContainer(data); err == nil {
			t.Fatalf("%s: expected error", tc.name)
		}
	}
}

func TestParseWebPContainer_Truncation(t *testing.T) {
	data := tinyLosslessWebP(t)
	for i := 0; i < len(data); i++ {
		if _, err := parseWebPContainer(data[:i]); err == nil {
			t.Fatalf("prefix of %d bytes accepted", i)
		}
	}
	// The complete file must parse.
	if _, err := parseWebPContainer(data); err != nil {
		t.Fatal(err)
	}
	// Trailing garbage beyond the declared RIFF size is tolerated.
	padded := append(append([]byte(nil), data...), 0xde, 0xad, 0xbe, 0xef)
	if _, err := parseWebPContainer(padded); err != nil {
		t.Fatalf("trailing bytes should be ignored: %v", err)
	}
}

func TestParseWebPContainer_SizeOverflow(t *testing.T) {
	good := tinyLosslessWebP(t)

	// RIFF size field claiming far more than available.
	data := append([]byte(nil), good...)
	binary.LittleEndian.PutUint32(data[4:8], 0xFFFFFFFF)
	if _, err := parseWebPContainer(data); err == nil {
		t.Fatal("huge RIFF size accepted")
	}

	// Chunk size field claiming far more than available.
	data = append([]byte(nil), good...)
	binary.LittleEndian.PutUint32(data[16:20], 0xFFFFFF00)
	if _, err := parseWebPContainer(data); err == nil {
		t.Fatal("huge chunk size accepted")
	}

	// RIFF size smaller than the form type.
	data = append([]byte(nil), good...)
	binary.LittleEndian.PutUint32(data[4:8], 3)
	if _, err := parseWebPContainer(data); err == nil {
		t.Fatal("tiny RIFF size accepted")
	}
}

func TestParseWebPContainer_OddChunkPadding(t *testing.T) {
	payload := []byte{1, 2, 3} // odd size: one pad byte must follow
	vp8l := tinyLosslessWebP(t)
	// Extract the VP8L chunk (fourcc+size+payload) from a real file.
	vp8lChunk := vp8l[12:]

	build := func(withPad bool) []byte {
		var body bytes.Buffer
		body.WriteString("WEBP")
		body.Write(vp8lChunk)
		body.WriteString("EXIF")
		var sz [4]byte
		binary.LittleEndian.PutUint32(sz[:], 3)
		body.Write(sz[:])
		body.Write(payload)
		if withPad {
			body.WriteByte(0) // RIFF pad byte for the odd-size chunk
		}
		var out bytes.Buffer
		out.WriteString("RIFF")
		binary.LittleEndian.PutUint32(sz[:], uint32(body.Len()))
		out.Write(sz[:])
		out.Write(body.Bytes())
		return out.Bytes()
	}

	chunks, err := parseWebPContainer(build(true))
	if err != nil {
		t.Fatalf("odd-size chunk with padding rejected: %v", err)
	}
	if len(chunks) != 2 || chunks[0].FourCC != "VP8L" {
		t.Fatalf("unexpected chunks: %+v", chunks)
	}
	if chunks[1].FourCC != "EXIF" || len(chunks[1].Payload) != 3 {
		t.Fatalf("padding misaligned chunk walk: %+v", chunks)
	}

	// A declared odd size without the pad byte counted in the RIFF size is
	// tolerated only at the very end of the file; a truncated chunk payload
	// in the middle must be rejected. Here the EXIF payload claims 3 bytes
	// but the file ends right after 2 of them.
	truncated := build(true)
	truncated = truncated[:len(truncated)-2]
	binary.LittleEndian.PutUint32(truncated[4:8], uint32(len(truncated)-8))
	if _, err := parseWebPContainer(truncated); err == nil {
		t.Fatal("truncated odd-size chunk accepted")
	}
}

func TestParseWebPContainer_VP8XRules(t *testing.T) {
	// VP8X without any image or animation chunk must be rejected.
	vp8x := make([]byte, 10)
	data := buildRIFF(t, [2][]byte{[]byte("VP8X"), vp8x})
	if _, err := parseWebPContainer(data); err == nil {
		t.Fatal("VP8X without image chunk accepted")
	}

	// VP8X with truncated payload must be rejected.
	data = buildRIFF(t, [2][]byte{[]byte("VP8X"), make([]byte, 4)})
	if _, err := parseWebPContainer(data); err == nil {
		t.Fatal("short VP8X payload accepted")
	}
}

func TestCheckDecodeBudget(t *testing.T) {
	if err := checkDecodeBudget(100, 100, 4); err != nil {
		t.Fatal(err)
	}
	if err := checkDecodeBudget(0, 100, 4); err == nil {
		t.Fatal("zero width accepted")
	}
	if err := checkDecodeBudget(-1, 100, 4); err == nil {
		t.Fatal("negative width accepted")
	}
	// 2^24 x 2^24 RGBA would overflow a 32-bit byte count; must be rejected
	// before any allocation is attempted.
	if err := checkDecodeBudget(1<<24, 1<<24, 4); err == nil {
		t.Fatal("oversize canvas accepted")
	}
	if err := checkDecodeBudget(maxDecodeBufferSize, 2, 1); err == nil {
		t.Fatal("over-budget buffer accepted")
	}
}

func TestValidEncodeArgs(t *testing.T) {
	pix := make([]byte, 4*10*10)
	if err := validEncodeArgs("test", pix, 10, 10, 40, 4); err != nil {
		t.Fatal(err)
	}
	// Stride larger than a row is fine if the buffer holds the last row.
	if err := validEncodeArgs("test", make([]byte, 9*50+40), 10, 10, 50, 4); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name                   string
		pix                    []byte
		w, h, stride, channels int
	}{
		{"empty pix", nil, 10, 10, 40, 4},
		{"zero width", pix, 0, 10, 40, 4},
		{"zero height", pix, 10, 0, 40, 4},
		{"negative height", pix, 10, -1, 40, 4},
		{"too wide", pix, maxWebpDimension + 1, 10, 4 * (maxWebpDimension + 1), 4},
		{"too tall", pix, 10, maxWebpDimension + 1, 40, 4},
		{"short stride", pix, 10, 10, 39, 4},
		{"short buffer", pix[:len(pix)-1], 10, 10, 40, 4},
		{"short buffer with stride", make([]byte, 9*50+39), 10, 10, 50, 4},
	}
	for _, tc := range cases {
		if err := validEncodeArgs("test", tc.pix, tc.w, tc.h, tc.stride, tc.channels); err == nil {
			t.Fatalf("%s: expected error", tc.name)
		}
	}
}
