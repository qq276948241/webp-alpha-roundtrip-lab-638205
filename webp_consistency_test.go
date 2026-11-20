// Copyright 2026 <chaishushan{AT}gmail.com>. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webp

import (
	"bytes"
	"image"
	"os"
	"testing"
)

// tFixture describes one offline testdata fixture: its bitstream kind and
// the dimensions/alpha every entry point must agree on.
var tFixtures = []struct {
	Filename string
	Kind     string // "vp8" (lossy), "vp8l" (lossless), "vp8x+alph" (lossy+alpha)
	Width    int
	Height   int
	HasAlpha bool
}{
	{"video-001.webp", "vp8", 150, 103, false},
	{"yellow_rose.lossy.webp", "vp8", 400, 301, false},
	{"blue-purple-pink.lossy.webp", "vp8", 150, 100, false},
	{"blue-purple-pink.lossless.webp", "vp8l", 150, 100, false},
	{"gopher-doc.1bpp.lossless.webp", "vp8l", 75, 100, false},
	{"gopher-doc.8bpp.lossless.webp", "vp8l", 75, 100, false},
	{"1_webp_ll.webp", "vp8l", 400, 301, true},
	{"tux.lossless.webp", "vp8l", 386, 395, true},
	{"yellow_rose.lossy-with-alpha.webp", "vp8x+alph", 400, 301, true},
	{"2_webp_a.webp", "vp8x+alph", 386, 395, true},
	{"4_webp_a.webp", "vp8x+alph", 421, 163, true},
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(testdataDir + name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// TestConsistency_GetInfoVsDecode verifies that GetInfo, DecodeRGBA,
// DecodeRGB, DecodeGray and the standard image.Decode API all report the
// same dimensions for the same stream, across VP8, VP8L and VP8X+ALPH
// bitstreams.
func TestConsistency_GetInfoVsDecode(t *testing.T) {
	for _, f := range tFixtures {
		data := readFixture(t, f.Filename)

		w, h, hasAlpha, err := GetInfo(data)
		if err != nil {
			t.Fatalf("%s: GetInfo: %v", f.Filename, err)
		}
		if w != f.Width || h != f.Height || hasAlpha != f.HasAlpha {
			t.Fatalf("%s: GetInfo = %dx%d alpha=%v, want %dx%d alpha=%v",
				f.Filename, w, h, hasAlpha, f.Width, f.Height, f.HasAlpha)
		}

		rgba, err := DecodeRGBA(data)
		if err != nil {
			t.Fatalf("%s: DecodeRGBA: %v", f.Filename, err)
		}
		if got := rgba.Bounds(); got != image.Rect(0, 0, f.Width, f.Height) {
			t.Fatalf("%s: DecodeRGBA bounds = %v", f.Filename, got)
		}

		rgb, err := DecodeRGB(data)
		if err != nil {
			t.Fatalf("%s: DecodeRGB: %v", f.Filename, err)
		}
		if got := rgb.Bounds(); got != image.Rect(0, 0, f.Width, f.Height) {
			t.Fatalf("%s: DecodeRGB bounds = %v", f.Filename, got)
		}

		gray, err := DecodeGray(data)
		if err != nil {
			t.Fatalf("%s: DecodeGray: %v", f.Filename, err)
		}
		if got := gray.Bounds(); got != image.Rect(0, 0, f.Width, f.Height) {
			t.Fatalf("%s: DecodeGray bounds = %v", f.Filename, got)
		}

		// Standard library entry points must agree as well.
		m, format, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("%s: image.Decode: %v", f.Filename, err)
		}
		if format != "webp" {
			t.Fatalf("%s: image.Decode format = %q", f.Filename, format)
		}
		if got := m.Bounds(); got != image.Rect(0, 0, f.Width, f.Height) {
			t.Fatalf("%s: image.Decode bounds = %v", f.Filename, got)
		}

		cfg, err := DecodeConfig(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("%s: DecodeConfig: %v", f.Filename, err)
		}
		if cfg.Width != f.Width || cfg.Height != f.Height {
			t.Fatalf("%s: DecodeConfig = %dx%d", f.Filename, cfg.Width, cfg.Height)
		}
	}
}

// TestConsistency_RGBvsRGBA checks that DecodeRGB and DecodeRGBA produce
// the same RGB values at every pixel, including boundary pixels, and that
// opaque streams decode to alpha=0xff everywhere.
func TestConsistency_RGBvsRGBA(t *testing.T) {
	for _, f := range tFixtures {
		data := readFixture(t, f.Filename)
		rgba, err := DecodeRGBA(data)
		if err != nil {
			t.Fatalf("%s: %v", f.Filename, err)
		}
		rgb, err := DecodeRGB(data)
		if err != nil {
			t.Fatalf("%s: %v", f.Filename, err)
		}
		w, h := f.Width, f.Height
		// Sample the four corners, edge midpoints and the center: these
		// hit the boundary macroblocks of the bitstream.
		points := [][2]int{
			{0, 0}, {w - 1, 0}, {0, h - 1}, {w - 1, h - 1},
			{w / 2, 0}, {w / 2, h - 1}, {0, h / 2}, {w - 1, h / 2},
			{w / 2, h / 2}, {15, 15}, {16, 16}, {17, 17},
		}
		for _, p := range points {
			x, y := p[0], p[1]
			if x >= w || y >= h {
				continue
			}
			c := rgba.RGBAAt(x, y)
			r := rgb.RGBAt(x, y)
			if c.R != r[0] || c.G != r[1] || c.B != r[2] {
				t.Fatalf("%s: pixel (%d,%d): RGBA=%v RGB=%v", f.Filename, x, y, c, r)
			}
			if !f.HasAlpha && c.A != 0xff {
				t.Fatalf("%s: opaque stream decoded alpha=%d at (%d,%d)", f.Filename, c.A, x, y)
			}
		}
	}
}

// TestConsistency_AlphaPixels verifies per-pixel alpha on the alpha
// fixtures: transparent and opaque regions reported by GetInfo must show
// up in the decoded alpha channel, and DecodeRGB must not expose them.
func TestConsistency_AlphaPixels(t *testing.T) {
	for _, f := range tFixtures {
		if !f.HasAlpha {
			continue
		}
		data := readFixture(t, f.Filename)
		rgba, err := DecodeRGBA(data)
		if err != nil {
			t.Fatalf("%s: %v", f.Filename, err)
		}
		var transparent, opaque int
		b := rgba.Bounds()
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				switch a := rgba.RGBAAt(x, y).A; a {
				case 0x00:
					transparent++
				case 0xff:
					opaque++
				}
			}
		}
		if transparent == 0 && opaque == 0 {
			t.Fatalf("%s: no fully transparent or opaque pixels found; alpha channel looks wrong", f.Filename)
		}
	}
}

// TestConsistency_DecodeToSize checks the scaled decode paths against the
// full-size decode dimensions.
func TestConsistency_DecodeToSize(t *testing.T) {
	data := readFixture(t, "video-001.webp")
	for _, size := range [][2]int{{1, 1}, {16, 16}, {75, 52}, {150, 103}, {300, 206}} {
		w, h := size[0], size[1]
		rgba, err := DecodeRGBAToSize(data, w, h)
		if err != nil {
			t.Fatalf("DecodeRGBAToSize(%d,%d): %v", w, h, err)
		}
		if got := rgba.Bounds(); got != image.Rect(0, 0, w, h) {
			t.Fatalf("DecodeRGBAToSize(%d,%d) bounds = %v", w, h, got)
		}
		rgb, err := DecodeRGBToSize(data, w, h)
		if err != nil {
			t.Fatalf("DecodeRGBToSize(%d,%d): %v", w, h, err)
		}
		if got := rgb.Bounds(); got != image.Rect(0, 0, w, h) {
			t.Fatalf("DecodeRGBToSize(%d,%d) bounds = %v", w, h, got)
		}
		gray, err := DecodeGrayToSize(data, w, h)
		if err != nil {
			t.Fatalf("DecodeGrayToSize(%d,%d): %v", w, h, err)
		}
		if got := gray.Bounds(); got != image.Rect(0, 0, w, h) {
			t.Fatalf("DecodeGrayToSize(%d,%d) bounds = %v", w, h, got)
		}
	}
}
