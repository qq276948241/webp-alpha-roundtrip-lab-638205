// Copyright 2026 <chaishushan{AT}gmail.com>. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webp

import (
	"bytes"
	"errors"
	"image"
	"io"
	"math/rand"
	"sync"
	"testing"
)

// TestRobust_EmptyInput: every entry point rejects empty input with an
// error, never a panic or a nil-error partial result.
func TestRobust_EmptyInput(t *testing.T) {
	for _, data := range [][]byte{nil, {}} {
		if _, _, _, err := GetInfo(data); err == nil {
			t.Fatal("GetInfo accepted empty input")
		}
		if m, err := DecodeRGBA(data); err == nil || m != nil {
			t.Fatal("DecodeRGBA accepted empty input")
		}
		if m, err := DecodeRGB(data); err == nil || m != nil {
			t.Fatal("DecodeRGB accepted empty input")
		}
		if m, err := DecodeGray(data); err == nil || m != nil {
			t.Fatal("DecodeGray accepted empty input")
		}
		if m, err := Decode(bytes.NewReader(data)); err == nil || m != nil {
			t.Fatal("Decode accepted empty input")
		}
		if _, err := DecodeConfig(bytes.NewReader(data)); err == nil {
			t.Fatal("DecodeConfig accepted empty input")
		}
		if _, err := GetMetadata(data, "EXIF"); err == nil {
			t.Fatal("GetMetadata accepted empty input")
		}
		if p, err := DecodeRGBAToSize(data, 4, 4); err == nil || p != nil {
			t.Fatal("DecodeRGBAToSize accepted empty input")
		}
	}
}

// TestRobust_GarbageInput: random and structured garbage must produce a
// stable error, never a nil error or a partial image.
func TestRobust_GarbageInput(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	for i := 0; i < 64; i++ {
		garbage := make([]byte, 16+rng.Intn(1024))
		rng.Read(garbage)
		if m, err := DecodeRGBA(garbage); err == nil || m != nil {
			t.Fatalf("%d: random garbage decoded", i)
		}
	}
	// Valid RIFF/WEBP header with garbage where the bitstream should be.
	header := []byte("RIFF\x12\x00\x00\x00WEBPVP8L\x05\x00\x00\x00")
	garbage := append(header, 0xff, 0xff, 0xff, 0xff, 0xff, 0x00)
	if m, err := DecodeRGBA(garbage); err == nil || m != nil {
		t.Fatal("garbage bitstream decoded")
	}
}

// TestRobust_Truncation: every strict prefix of a valid file must fail
// cleanly on every decode entry point.
func TestRobust_Truncation(t *testing.T) {
	data := readFixture(t, "gopher-doc.1bpp.lossless.webp") // 442 bytes
	for i := 0; i < len(data); i++ {
		prefix := data[:i]
		if m, err := DecodeRGBA(prefix); err == nil || m != nil {
			t.Fatalf("DecodeRGBA accepted %d-byte prefix", i)
		}
		if m, err := DecodeRGB(prefix); err == nil || m != nil {
			t.Fatalf("DecodeRGB accepted %d-byte prefix", i)
		}
		if m, err := DecodeGray(prefix); err == nil || m != nil {
			t.Fatalf("DecodeGray accepted %d-byte prefix", i)
		}
	}
	// Sanity: the complete file decodes.
	if _, err := DecodeRGBA(data); err != nil {
		t.Fatal(err)
	}
}

// TestRobust_CorruptStream: targeted corruption of a valid stream must
// fail, and repeated attempts must fail with the same error (stable
// behavior, no state leakage between calls).
func TestRobust_CorruptStream(t *testing.T) {
	base := readFixture(t, "blue-purple-pink.lossless.webp")

	corruptions := map[string]func(d []byte){
		"riff-magic":   func(d []byte) { d[0] = 'X' },
		"webp-magic":   func(d []byte) { d[8] = 'X' },
		"riff-size+":   func(d []byte) { d[4] += 0x40 },
		"chunk-fourcc": func(d []byte) { d[12] = 'X' },
		"chunk-size":   func(d []byte) { d[16] = 0xff; d[17] = 0xff; d[18] = 0xff; d[19] = 0xff },
		"vp8l-sig":     func(d []byte) { d[20] = 0x00 }, // VP8L signature byte must be 0x2f
	}
	for name, corrupt := range corruptions {
		data := append([]byte(nil), base...)
		corrupt(data)

		var first error
		for attempt := 0; attempt < 3; attempt++ {
			m, err := DecodeRGBA(data)
			if err == nil || m != nil {
				t.Fatalf("%s: corrupted stream decoded (attempt %d)", name, attempt)
			}
			if first == nil {
				first = err
			} else if err.Error() != first.Error() {
				t.Fatalf("%s: unstable error: %q then %q", name, first, err)
			}
		}
		// The corruption must not have touched the source.
		if _, err := DecodeRGBA(base); err != nil {
			t.Fatalf("%s: base fixture no longer decodes: %v", name, err)
		}
	}
}

// TestRobust_DecodeToSizeBudget: over-budget and nonsensical scale
// requests are rejected before any buffer is allocated.
func TestRobust_DecodeToSizeBudget(t *testing.T) {
	data := readFixture(t, "video-001.webp")
	for _, size := range [][2]int{
		{0, 10}, {10, 0}, {-1, 10}, {10, -1},
		{1 << 20, 1 << 20},       // 4 TiB of RGBA: way over budget
		{maxDecodeBufferSize, 2}, // over budget even at 1 channel
	} {
		w, h := size[0], size[1]
		if p, err := DecodeRGBAToSize(data, w, h); err == nil || p != nil {
			t.Fatalf("DecodeRGBAToSize(%d,%d) accepted", w, h)
		}
		if p, err := DecodeRGBToSize(data, w, h); err == nil || p != nil {
			t.Fatalf("DecodeRGBToSize(%d,%d) accepted", w, h)
		}
		if p, err := DecodeGrayToSize(data, w, h); err == nil || p != nil {
			t.Fatalf("DecodeGrayToSize(%d,%d) accepted", w, h)
		}
	}
}

// TestRobust_EncodeArgValidation: the encode layer rejects impossible
// geometry and buffers before touching C (in-package API surface).
func TestRobust_EncodeArgValidation(t *testing.T) {
	pix := make([]byte, 4*10*10)
	if _, err := webpEncodeRGBA(pix, 10, 10, 40, 90); err != nil {
		t.Fatal(err)
	}
	if _, err := webpEncodeRGBA(pix[:len(pix)-1], 10, 10, 40, 90); err == nil {
		t.Fatal("short pixel buffer accepted")
	}
	if _, err := webpEncodeRGBA(pix, 10, 10, 39, 90); err == nil {
		t.Fatal("short stride accepted")
	}
	if _, err := webpEncodeRGBA(pix, maxWebpDimension+1, 1, 4*(maxWebpDimension+1), 90); err == nil {
		t.Fatal("over-limit width accepted")
	}
	if _, err := webpEncodeRGBA(pix, 10, 10, 40, 101); err == nil {
		t.Fatal("quality 101 accepted")
	}
	if _, err := webpEncodeRGB(pix, 10, 10, 29, 90); err == nil {
		t.Fatal("RGB stride below 3*width accepted")
	}
	if _, err := webpEncodeLosslessRGBA(0, pix[:len(pix)-4], 10, 10, 40); err == nil {
		t.Fatal("lossless RGBA with short buffer accepted")
	}
}

// TestRobust_ShortReads: readers that return data one byte at a time must
// still decode and report config correctly.
func TestRobust_ShortReads(t *testing.T) {
	data := readFixture(t, "video-001.webp")

	m, err := Decode(io.MultiReader(bytes.NewReader(nil), &oneByteReader{data: data}))
	if err != nil {
		t.Fatalf("Decode with short reads: %v", err)
	}
	if got := m.Bounds(); got.Dx() != 150 || got.Dy() != 103 {
		t.Fatalf("bounds = %v", got)
	}

	cfg, err := DecodeConfig(&oneByteReader{data: data})
	if err != nil {
		t.Fatalf("DecodeConfig with short reads: %v", err)
	}
	if cfg.Width != 150 || cfg.Height != 103 {
		t.Fatalf("config = %dx%d", cfg.Width, cfg.Height)
	}
}

type oneByteReader struct {
	data []byte
	pos  int
}

func (r *oneByteReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	if len(p) == 0 {
		return 0, nil
	}
	p[0] = r.data[r.pos]
	r.pos++
	return 1, nil
}

// TestRobust_ErrorReaders: readers that fail must surface the error.
func TestRobust_ErrorReaders(t *testing.T) {
	errBoom := errors.New("boom")

	if _, err := Decode(&failAfterReader{n: 10, err: errBoom}); !errors.Is(err, errBoom) {
		t.Fatalf("Decode error = %v, want boom", err)
	}
	if _, err := DecodeConfig(&failAfterReader{n: 0, err: errBoom}); !errors.Is(err, errBoom) {
		t.Fatalf("DecodeConfig error = %v, want boom", err)
	}
	// A reader that silently truncates must not decode successfully.
	data := readFixture(t, "video-001.webp")
	if _, err := Decode(&failAfterReader{n: 100, data: data, err: io.EOF}); err == nil {
		t.Fatal("Decode accepted silently truncated stream")
	}
}

type failAfterReader struct {
	n    int // bytes to serve before failing
	data []byte
	err  error
}

func (r *failAfterReader) Read(p []byte) (int, error) {
	if r.n <= 0 {
		return 0, r.err
	}
	if len(r.data) == 0 {
		// Serve zeros.
		if len(p) > r.n {
			p = p[:r.n]
		}
		for i := range p {
			p[i] = 0
		}
		r.n -= len(p)
		return len(p), nil
	}
	if len(p) > r.n {
		p = p[:r.n]
	}
	if len(p) > len(r.data) {
		p = p[:len(r.data)]
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	r.n -= n
	return n, nil
}

// TestRobust_BufferIndependence: two decodes of the same data must return
// independent buffers; mutating one must not affect the other, and the
// caller-owned input must not be modified either.
func TestRobust_BufferIndependence(t *testing.T) {
	data := readFixture(t, "1_webp_ll.webp")
	snapshot := append([]byte(nil), data...)

	m1, err := DecodeRGBA(data)
	if err != nil {
		t.Fatal(err)
	}
	m2, err := DecodeRGBA(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(m1.Pix) == 0 || len(m2.Pix) == 0 {
		t.Fatal("empty pix")
	}
	if &m1.Pix[0] == &m2.Pix[0] {
		t.Fatal("decodes share a pixel buffer")
	}
	for i := range m1.Pix {
		m1.Pix[i] ^= 0xff
	}
	assertSamePixels(t, m2, mustDecodeRGBA(t, data))
	if !bytes.Equal(data, snapshot) {
		t.Fatal("decode modified the input buffer")
	}
}

func mustDecodeRGBA(t *testing.T, data []byte) *image.RGBA {
	t.Helper()
	m, err := DecodeRGBA(data)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// TestRobust_Concurrency: concurrent use of every entry point from many
// goroutines must be isolated: results identical to serial execution.
// Run with `go test -race` to also catch data races.
func TestRobust_Concurrency(t *testing.T) {
	lossy := readFixture(t, "video-001.webp")
	lossless := readFixture(t, "1_webp_ll.webp")
	withExif := readFixture(t, "photo.lossy.webp")

	// Serial references.
	refW, refH, refAlpha, err := GetInfo(lossless)
	if err != nil {
		t.Fatal(err)
	}
	refRGBA, err := DecodeRGBA(lossless)
	if err != nil {
		t.Fatal(err)
	}
	refExif, err := GetMetadata(withExif, "EXIF")
	if err != nil {
		t.Fatal(err)
	}
	src := newPatternRGBA(31, 17)
	refEncoded, err := EncodeExactLosslessRGBA(src)
	if err != nil {
		t.Fatal(err)
	}

	const workers = 16
	var wg sync.WaitGroup
	errs := make(chan error, workers*4)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for iter := 0; iter < 8; iter++ {
				w, h, a, err := GetInfo(lossless)
				if err != nil || w != refW || h != refH || a != refAlpha {
					errs <- errors.New("GetInfo mismatch")
					return
				}
				m, err := DecodeRGBA(lossless)
				if err != nil {
					errs <- err
					return
				}
				if !bytes.Equal(m.Pix, refRGBA.Pix) {
					errs <- errors.New("DecodeRGBA mismatch")
					return
				}
				if _, err := DecodeRGB(lossy); err != nil {
					errs <- err
					return
				}
				exif, err := GetMetadata(withExif, "EXIF")
				if err != nil || !bytes.Equal(exif, refExif) {
					errs <- errors.New("GetMetadata mismatch")
					return
				}
				enc, err := EncodeExactLosslessRGBA(src)
				if err != nil || !bytes.Equal(enc, refEncoded) {
					errs <- errors.New("Encode mismatch")
					return
				}
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}
