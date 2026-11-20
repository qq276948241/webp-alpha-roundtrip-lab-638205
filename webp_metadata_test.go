// Copyright 2026 <chaishushan{AT}gmail.com>. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webp

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// chunkFourCCs returns the chunk four-character codes of a WebP file in
// file order, as seen by the shared container parser.
func chunkFourCCs(t *testing.T, data []byte) []string {
	t.Helper()
	chunks, err := parseWebPContainer(data)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, c := range chunks {
		out = append(out, c.FourCC)
	}
	return out
}

func countChunk(fourccs []string, want string) int {
	n := 0
	for _, f := range fourccs {
		if f == want {
			n++
		}
	}
	return n
}

func indexOfChunk(fourccs []string, want string) int {
	for i, f := range fourccs {
		if f == want {
			return i
		}
	}
	return -1
}

// TestMetadata_RoundTrip sets and reads back each metadata format,
// verifying the raw payload bytes survive unchanged — including an
// odd-length payload that forces RIFF padding.
func TestMetadata_RoundTrip(t *testing.T) {
	base := readFixture(t, "blue-purple-pink.lossless.webp")

	payloads := map[string][]byte{
		"EXIF": []byte("Exif\x00\x00II*\x00\x08\x00\x00\x00\x00\x00\x00"), // 19 bytes: odd
		"ICCP": []byte("fake-icc-profile\x00\x01\x02\x03\x04"),
		"XMP":  []byte("<x:xmpmeta xmlns:x='adobe:ns:meta/'></x:xmpmeta>"),
	}
	for format, payload := range payloads {
		withMeta, err := SetMetadata(base, payload, format)
		if err != nil {
			t.Fatalf("%s: SetMetadata: %v", format, err)
		}
		got, err := GetMetadata(withMeta, format)
		if err != nil {
			t.Fatalf("%s: GetMetadata: %v", format, err)
		}
		if !bytes.Equal(got, payload) {
			t.Fatalf("%s: payload changed: got %d bytes, want %d", format, len(got), len(payload))
		}
		// The image itself must still decode to the same pixels.
		m0, err := DecodeRGBA(base)
		if err != nil {
			t.Fatalf("%s: decode base: %v", format, err)
		}
		m1, err := DecodeRGBA(withMeta)
		if err != nil {
			t.Fatalf("%s: decode with metadata: %v", format, err)
		}
		assertSamePixels(t, m0, m1)
		// Format names are case-insensitive.
		if _, err := GetMetadata(withMeta, "exif"); format == "EXIF" && err != nil {
			t.Fatalf("lowercase format rejected: %v", err)
		}
	}
}

// TestMetadata_ChunkOrder verifies that setting all three metadata formats
// yields the canonical chunk order: VP8X first, ICCP before the image
// chunk, EXIF and XMP after it.
func TestMetadata_ChunkOrder(t *testing.T) {
	base := readFixture(t, "blue-purple-pink.lossless.webp")
	data := base
	var err error
	if data, err = SetMetadata(data, []byte("xmp-payload"), "XMP"); err != nil {
		t.Fatal(err)
	}
	if data, err = SetMetadata(data, []byte("exif-payload"), "EXIF"); err != nil {
		t.Fatal(err)
	}
	if data, err = SetMetadata(data, []byte("iccp-payload"), "ICCP"); err != nil {
		t.Fatal(err)
	}

	fourccs := chunkFourCCs(t, data)
	if fourccs[0] != "VP8X" {
		t.Fatalf("first chunk = %q, want VP8X: %v", fourccs[0], fourccs)
	}
	iccp := indexOfChunk(fourccs, "ICCP")
	exif := indexOfChunk(fourccs, "EXIF")
	xmp := indexOfChunk(fourccs, "XMP ")
	img := indexOfChunk(fourccs, "VP8L")
	if iccp < 0 || exif < 0 || xmp < 0 || img < 0 {
		t.Fatalf("missing chunks: %v", fourccs)
	}
	if !(iccp < img && img < exif && img < xmp) {
		t.Fatalf("bad chunk order: %v", fourccs)
	}
}

// TestMetadata_DuplicateReplaced verifies the duplicate-chunk policy:
// setting the same format twice replaces the previous chunk instead of
// appending a second one.
func TestMetadata_DuplicateReplaced(t *testing.T) {
	base := readFixture(t, "blue-purple-pink.lossless.webp")
	data, err := SetMetadata(base, []byte("first"), "EXIF")
	if err != nil {
		t.Fatal(err)
	}
	data, err = SetMetadata(data, []byte("second"), "EXIF")
	if err != nil {
		t.Fatal(err)
	}
	got, err := GetMetadata(data, "EXIF")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "second" {
		t.Fatalf("GetMetadata = %q, want %q", got, "second")
	}
	if n := countChunk(chunkFourCCs(t, data), "EXIF"); n != 1 {
		t.Fatalf("found %d EXIF chunks, want 1", n)
	}
}

// TestMetadata_UnknownChunkPreserved verifies the unknown-chunk policy:
// chunks this package does not understand survive a SetMetadata
// round-trip untouched.
func TestMetadata_UnknownChunkPreserved(t *testing.T) {
	base := readFixture(t, "blue-purple-pink.lossless.webp")

	// Splice an unknown ABCD chunk (odd length, to also exercise padding)
	// after the image chunk and fix up the RIFF size.
	unknown := []byte("unknown-chunk-payload!") // 22 bytes
	var out bytes.Buffer
	out.Write(base)
	out.WriteString("ABCD")
	var sz [4]byte
	binary.LittleEndian.PutUint32(sz[:], uint32(len(unknown)))
	out.Write(sz[:])
	out.Write(unknown)
	if len(unknown)%2 != 0 {
		out.WriteByte(0)
	}
	withUnknown := out.Bytes()
	binary.LittleEndian.PutUint32(withUnknown[4:8], uint32(len(withUnknown)-8))

	if _, err := parseWebPContainer(withUnknown); err != nil {
		t.Fatalf("spliced file invalid: %v", err)
	}
	data, err := SetMetadata(withUnknown, []byte("exif"), "EXIF")
	if err != nil {
		t.Fatal(err)
	}
	fourccs := chunkFourCCs(t, data)
	if countChunk(fourccs, "ABCD") != 1 {
		t.Fatalf("unknown chunk lost: %v", fourccs)
	}
}

// TestMetadata_Missing verifies that querying metadata that is not present
// is an error, never a nil-error empty result.
func TestMetadata_Missing(t *testing.T) {
	base := readFixture(t, "blue-purple-pink.lossless.webp")
	for _, format := range []string{"EXIF", "ICCP", "XMP"} {
		if _, err := GetMetadata(base, format); err == nil {
			t.Fatalf("%s: missing metadata returned nil error", format)
		}
	}
}

// TestMetadata_Fixture reads the EXIF chunk shipped in photo.lossy.webp
// and moves it to another file byte-for-byte.
func TestMetadata_Fixture(t *testing.T) {
	photo := readFixture(t, "photo.lossy.webp")
	exif, err := GetMetadata(photo, "EXIF")
	if err != nil {
		t.Fatal(err)
	}
	if len(exif) == 0 {
		t.Fatal("empty EXIF in photo.lossy.webp")
	}

	base := readFixture(t, "blue-purple-pink.lossless.webp")
	data, err := SetMetadata(base, exif, "EXIF")
	if err != nil {
		t.Fatal(err)
	}
	got, err := GetMetadata(data, "EXIF")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, exif) {
		t.Fatal("EXIF payload changed in transit")
	}
}

// TestMetadata_BadInput verifies metadata entry points reject bad input
// with an error (and no partial result).
func TestMetadata_BadInput(t *testing.T) {
	base := readFixture(t, "blue-purple-pink.lossless.webp")

	if _, err := GetMetadata(nil, "EXIF"); err == nil {
		t.Fatal("nil data accepted")
	}
	if _, err := GetMetadata(base, "NOPE"); err == nil {
		t.Fatal("unknown format accepted")
	}
	if _, err := GetMetadata(base[:20], "EXIF"); err == nil {
		t.Fatal("truncated data accepted")
	}
	if _, err := SetMetadata(base, nil, "EXIF"); err == nil {
		t.Fatal("nil metadata accepted")
	}
	if _, err := SetMetadata(base[:20], []byte("x"), "EXIF"); err == nil {
		t.Fatal("truncated data accepted by SetMetadata")
	}
	if _, err := SetMetadata(base, []byte("x"), "NOPE"); err == nil {
		t.Fatal("unknown format accepted by SetMetadata")
	}
}
