// Copyright 2026 <chaishushan{AT}gmail.com>. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webp

import (
	"image"
	"image/color"
	"math/rand"
	"testing"
)

// newUniformRGBA returns a w x h RGBA image filled with a single color.
func newUniformRGBA(w, h int, a uint8) *image.RGBA {
	m := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(m.Pix); i += 4 {
		m.Pix[i+0] = 0x7f
		m.Pix[i+1] = 0x3f
		m.Pix[i+2] = 0x9f
		m.Pix[i+3] = a
	}
	return m
}

// newPatternRGBA returns a w x h RGBA image with a deterministic
// pseudo-random pattern (including alpha), so round-trip tests exercise
// every channel at every pixel.
func newPatternRGBA(w, h int) *image.RGBA {
	rng := rand.New(rand.NewSource(int64(w)*1e6 + int64(h)))
	m := image.NewRGBA(image.Rect(0, 0, w, h))
	rng.Read(m.Pix)
	return m
}

// newPatternRGB returns a w x h RGBImage with a deterministic pattern.
func newPatternRGB(w, h int) *RGBImage {
	rng := rand.New(rand.NewSource(int64(w)*1e6 + int64(h) + 7))
	m := NewRGBImage(image.Rect(0, 0, w, h))
	rng.Read(m.XPix)
	return m
}

// assertSamePixels fails the test if the two images differ at any pixel,
// comparing via the color.Color interface (16-bit alpha-premultiplied
// values), so it works across image types.
func assertSamePixels(t *testing.T, want, got image.Image) {
	t.Helper()
	b := want.Bounds()
	if got.Bounds() != b {
		t.Fatalf("bounds differ: want %v, got %v", b, got.Bounds())
	}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			wr, wg, wb, wa := want.At(x, y).RGBA()
			gr, gg, gb, ga := got.At(x, y).RGBA()
			if wr != gr || wg != gg || wb != gb || wa != ga {
				t.Fatalf("pixel (%d,%d): want %04x %04x %04x %04x, got %04x %04x %04x %04x",
					x, y, wr, wg, wb, wa, gr, gg, gb, ga)
			}
		}
	}
}

// maxChannelDelta returns the largest per-channel 8-bit difference between
// two same-bounds images.
func maxChannelDelta(want, got image.Image) int {
	b := want.Bounds()
	max := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			w := color.RGBAModel.Convert(want.At(x, y)).(color.RGBA)
			g := color.RGBAModel.Convert(got.At(x, y)).(color.RGBA)
			for _, d := range []int{
				absDiff(int(w.R), int(g.R)),
				absDiff(int(w.G), int(g.G)),
				absDiff(int(w.B), int(g.B)),
				absDiff(int(w.A), int(g.A)),
			} {
				if d > max {
					max = d
				}
			}
		}
	}
	return max
}

func absDiff(a, b int) int {
	if a < b {
		return b - a
	}
	return a - b
}
