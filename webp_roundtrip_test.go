// Copyright 2026 <chaishushan{AT}gmail.com>. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webp

import (
	"bytes"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

// TestRoundTrip_LosslessExact encodes deterministic patterns (including
// odd widths/heights and boundary sizes around the 16x16 macroblock grid)
// with the exact lossless encoder and checks every pixel of every channel,
// alpha included.
func TestRoundTrip_LosslessExact(t *testing.T) {
	sizes := [][2]int{
		{1, 1}, {3, 1}, {1, 3}, {5, 7}, {7, 5},
		{15, 15}, {16, 16}, {17, 13}, {31, 9}, {64, 48},
	}
	for _, size := range sizes {
		w, h := size[0], size[1]
		src := newPatternRGBA(w, h)
		data, err := EncodeExactLosslessRGBA(src)
		if err != nil {
			t.Fatalf("%dx%d: %v", w, h, err)
		}
		got, err := DecodeRGBA(data)
		if err != nil {
			t.Fatalf("%dx%d: %v", w, h, err)
		}
		assertSamePixels(t, src, got)
	}
}

// TestRoundTrip_LosslessRGB checks lossless RGB round-trips at odd sizes.
func TestRoundTrip_LosslessRGB(t *testing.T) {
	sizes := [][2]int{{1, 1}, {3, 5}, {16, 16}, {17, 11}, {63, 33}}
	for _, size := range sizes {
		w, h := size[0], size[1]
		src := newPatternRGB(w, h)
		data, err := EncodeLosslessRGB(src)
		if err != nil {
			t.Fatalf("%dx%d: %v", w, h, err)
		}
		got, err := DecodeRGB(data)
		if err != nil {
			t.Fatalf("%dx%d: %v", w, h, err)
		}
		assertSamePixels(t, src, got)
	}
}

// TestRoundTrip_LosslessGray checks lossless Gray round-trips. The Gray
// decode path goes through the YUV colorspace (limited range), so the
// comparison uses a per-pixel tolerance instead of exact equality.
func TestRoundTrip_LosslessGray(t *testing.T) {
	src := image.NewGray(image.Rect(0, 0, 37, 23))
	for i := range src.Pix {
		src.Pix[i] = uint8(i * 7)
	}
	data, err := EncodeLosslessGray(src)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeGray(data)
	if err != nil {
		t.Fatal(err)
	}
	if got.Bounds() != src.Bounds() {
		t.Fatalf("bounds = %v", got.Bounds())
	}
	if d := maxChannelDelta(src, got); d > 26 {
		t.Fatalf("max channel delta = %d, want <= 26 (YUV limited range)", d)
	}
}

// TestRoundTrip_HiddenColorsExact verifies that with Exact the RGB values
// of fully transparent pixels survive the lossless round-trip unchanged.
func TestRoundTrip_HiddenColorsExact(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 9, 9))
	for i := 0; i < len(src.Pix); i += 4 {
		src.Pix[i+0] = uint8(i)
		src.Pix[i+1] = uint8(i >> 2)
		src.Pix[i+2] = uint8(i >> 4)
		src.Pix[i+3] = 0 // fully transparent, hidden color must be kept
	}
	data, err := EncodeExactLosslessRGBA(src)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeRGBA(data)
	if err != nil {
		t.Fatal(err)
	}
	assertSamePixels(t, src, got)
}

// TestRoundTrip_HiddenColorsNotLeaking verifies that hidden colors of
// transparent pixels never contaminate visible pixels: with the default
// (non-exact) lossless encoder and with the lossy encoder, fully
// transparent pixels stay fully transparent and opaque pixels keep their
// exact (lossless) or near (lossy) color.
func TestRoundTrip_HiddenColorsNotLeaking(t *testing.T) {
	// Left half opaque red, right half fully transparent with extreme
	// hidden colors that would be visible if they leaked.
	w, h := 32, 32
	src := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := src.PixOffset(x, y)
			if x < w/2 {
				src.Pix[i+0], src.Pix[i+1], src.Pix[i+2], src.Pix[i+3] = 0xff, 0x00, 0x00, 0xff
			} else {
				src.Pix[i+0], src.Pix[i+1], src.Pix[i+2], src.Pix[i+3] = 0x00, 0xff, 0x00, 0x00
			}
		}
	}

	t.Run("lossless", func(t *testing.T) {
		data, err := EncodeLosslessRGBA(src)
		if err != nil {
			t.Fatal(err)
		}
		got, err := DecodeRGBA(data)
		if err != nil {
			t.Fatal(err)
		}
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				c := got.RGBAAt(x, y)
				if x < w/2 {
					if c.R != 0xff || c.G != 0 || c.B != 0 || c.A != 0xff {
						t.Fatalf("visible pixel (%d,%d) contaminated: %v", x, y, c)
					}
				} else if c.A != 0 {
					t.Fatalf("transparent pixel (%d,%d) gained alpha %d", x, y, c.A)
				}
			}
		}
	})

	t.Run("lossy", func(t *testing.T) {
		data, err := EncodeRGBA(src, 90)
		if err != nil {
			t.Fatal(err)
		}
		got, err := DecodeRGBA(data)
		if err != nil {
			t.Fatal(err)
		}
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				c := got.RGBAAt(x, y)
				if x < w/2 {
					// Lossy edge ringing is allowed, but a pixel deep
					// inside the opaque half must stay red.
					if x < w/2-4 && (absDiff(int(c.R), 0xff) > 24 || c.G > 24 || c.B > 24 || c.A != 0xff) {
						t.Fatalf("visible pixel (%d,%d) contaminated: %v", x, y, c)
					}
				} else if x > w/2+4 && c.A != 0 {
					t.Fatalf("transparent pixel (%d,%d) gained alpha %d", x, y, c.A)
				}
			}
		}
	})
}

// TestRoundTrip_EncodeRGBvsEncode checks that the EncodeRGB fast path and
// the generic Encode entry produce cross-decodable, equivalent output for
// the same image and quality.
func TestRoundTrip_EncodeRGBvsEncode(t *testing.T) {
	src := newPatternRGB(41, 27)

	viaEncodeRGB, err := EncodeRGB(src, 90)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := Encode(&buf, src, &Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	viaEncode := buf.Bytes()

	// Same input, same quality, same code path: the bytes must be identical.
	if !bytes.Equal(viaEncodeRGB, viaEncode) {
		t.Fatal("EncodeRGB and Encode produced different bytes")
	}

	// And both must decode to the same pixels.
	m0, err := DecodeRGB(viaEncodeRGB)
	if err != nil {
		t.Fatal(err)
	}
	m1, err := DecodeRGB(viaEncode)
	if err != nil {
		t.Fatal(err)
	}
	assertSamePixels(t, m0, m1)

	// Lossy quality 90 on a smooth gradient: sanity-check the decode stays
	// close to the source, per the lossy contract. (A random pattern is
	// incompressible and legitimately deviates far more.)
	gradient := NewRGBImage(image.Rect(0, 0, 41, 27))
	for y := 0; y < 27; y++ {
		for x := 0; x < 41; x++ {
			gradient.SetRGB(x, y, [3]uint8{uint8(x * 6), uint8(y * 9), uint8(x*3 + y*3)})
		}
	}
	gradientData, err := EncodeRGB(gradient, 90)
	if err != nil {
		t.Fatal(err)
	}
	gradientDecoded, err := DecodeRGB(gradientData)
	if err != nil {
		t.Fatal(err)
	}
	if got := averageDelta(gradient, gradientDecoded); got > 8 {
		t.Fatalf("average delta too high: %d", got)
	}
}

// TestRoundTrip_NonZeroBounds verifies that images whose bounds do not
// start at (0,0) encode their visible rectangle, not their backing array.
func TestRoundTrip_NonZeroBounds(t *testing.T) {
	src := image.NewRGBA(image.Rect(5, 7, 5+17, 7+13))
	for y := 7; y < 7+13; y++ {
		for x := 5; x < 5+17; x++ {
			src.SetRGBA(x, y, color.RGBA{uint8(x * 3), uint8(y * 5), uint8(x + y), 0xff})
		}
	}
	data, err := EncodeLosslessRGBA(src)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeRGBA(data)
	if err != nil {
		t.Fatal(err)
	}
	if b := got.Bounds(); b != image.Rect(0, 0, 17, 13) {
		t.Fatalf("decoded bounds = %v, want 17x13", b)
	}
	for y := 0; y < 13; y++ {
		for x := 0; x < 17; x++ {
			want := src.RGBAAt(x+5, y+7)
			if c := got.RGBAAt(x, y); c != want {
				t.Fatalf("pixel (%d,%d): got %v, want %v", x, y, c, want)
			}
		}
	}
}

// TestRoundTrip_SubImage verifies that encoding a sub-image encodes only
// the sub-rectangle, even though it shares the parent's backing array.
func TestRoundTrip_SubImage(t *testing.T) {
	parent := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for i := range parent.Pix {
		parent.Pix[i] = uint8(i * 3)
	}
	sub := parent.SubImage(image.Rect(4, 4, 20, 12)).(*image.RGBA)

	data, err := EncodeLosslessRGBA(sub)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeRGBA(data)
	if err != nil {
		t.Fatal(err)
	}
	if b := got.Bounds(); b != image.Rect(0, 0, 16, 8) {
		t.Fatalf("decoded bounds = %v, want 16x8", b)
	}
	for y := 0; y < 8; y++ {
		for x := 0; x < 16; x++ {
			want := sub.RGBAAt(x+4, y+4)
			if c := got.RGBAAt(x, y); c != want {
				t.Fatalf("pixel (%d,%d): got %v, want %v", x, y, c, want)
			}
		}
	}
}

// TestRoundTrip_ColorModelConversion exercises the RGB/RGBA conversion
// rules for the non-RGBA source image types the encoder accepts.
func TestRoundTrip_ColorModelConversion(t *testing.T) {
	rgba64 := image.NewRGBA64(image.Rect(0, 0, 9, 5))
	nrgba := image.NewNRGBA(image.Rect(0, 0, 9, 5))
	gray16 := image.NewGray16(image.Rect(0, 0, 9, 5))
	ycbcr := image.NewYCbCr(image.Rect(0, 0, 16, 16), image.YCbCrSubsampleRatio420)
	for y := 0; y < 5; y++ {
		for x := 0; x < 9; x++ {
			c := color.RGBA{uint8(x * 20), uint8(y * 40), 0x80, 0xff}
			rgba64.SetRGBA64(x, y, color.RGBA64Model.Convert(c).(color.RGBA64))
			nrgba.SetNRGBA(x, y, color.NRGBAModel.Convert(c).(color.NRGBA))
			gray16.SetGray16(x, y, color.Gray16Model.Convert(c).(color.Gray16))
		}
	}

	for name, m := range map[string]image.Image{
		"RGBA64": rgba64,
		"NRGBA":  nrgba,
		"Gray16": gray16,
		"YCbCr":  ycbcr,
	} {
		var buf bytes.Buffer
		if err := Encode(&buf, m, &Options{Lossless: true}); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		got, err := Decode(&buf)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got.Bounds().Dx() != m.Bounds().Dx() || got.Bounds().Dy() != m.Bounds().Dy() {
			t.Fatalf("%s: bounds = %v, want %v", name, got.Bounds(), m.Bounds())
		}
	}
}

// TestEncode_RejectsBadInput covers the encode-side rejection rules:
// empty images, out-of-range quality, nil image.
func TestEncode_RejectsBadInput(t *testing.T) {
	empty := image.NewRGBA(image.Rect(0, 0, 0, 0))
	if _, err := EncodeRGBA(empty, 90); err == nil {
		t.Fatal("empty image accepted by EncodeRGBA")
	}
	if _, err := EncodeLosslessRGBA(empty); err == nil {
		t.Fatal("empty image accepted by EncodeLosslessRGBA")
	}
	if _, err := EncodeRGB(NewRGBImage(image.Rect(0, 0, 0, 5)), 90); err == nil {
		t.Fatal("zero-width image accepted by EncodeRGB")
	}

	m := newUniformRGBA(4, 4, 0xff)
	for _, q := range []float32{-1, -0.5, 100.5, 101, 1e9} {
		if _, err := EncodeRGBA(m, q); err == nil {
			t.Fatalf("quality %v accepted", q)
		}
		var buf bytes.Buffer
		if err := Encode(&buf, m, &Options{Quality: q}); err == nil {
			t.Fatalf("quality %v accepted by Encode", q)
		}
	}
	var buf bytes.Buffer
	if err := Encode(&buf, nil, nil); err == nil {
		t.Fatal("nil image accepted")
	}
}

// TestEncode_SaveCleanup verifies that a failed Save does not leave a
// partial file behind and that a successful Save round-trips.
func TestEncode_SaveCleanup(t *testing.T) {
	dir := t.TempDir()

	bad := filepath.Join(dir, "bad.webp")
	if err := Save(bad, newUniformRGBA(4, 4, 0xff), &Options{Quality: 200}); err == nil {
		t.Fatal("Save with invalid quality succeeded")
	}
	if _, err := os.Stat(bad); !os.IsNotExist(err) {
		t.Fatal("failed Save left a file behind")
	}

	good := filepath.Join(dir, "good.webp")
	src := newPatternRGBA(23, 17)
	if err := Save(good, src, &Options{Lossless: true, Exact: true}); err != nil {
		t.Fatal(err)
	}
	got, err := Load(good)
	if err != nil {
		t.Fatal(err)
	}
	assertSamePixels(t, src, got)

	cfg, err := LoadConfig(good)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Width != 23 || cfg.Height != 17 {
		t.Fatalf("LoadConfig = %dx%d", cfg.Width, cfg.Height)
	}
}

// TestEncode_LosslessFixtureRoundTrip re-encodes a real lossless fixture
// and checks the pixels survive bit-exactly.
func TestEncode_LosslessFixtureRoundTrip(t *testing.T) {
	data := readFixture(t, "blue-purple-pink.lossless.webp")
	src, err := DecodeRGBA(data)
	if err != nil {
		t.Fatal(err)
	}
	reencoded, err := EncodeExactLosslessRGBA(src)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeRGBA(reencoded)
	if err != nil {
		t.Fatal(err)
	}
	assertSamePixels(t, src, got)
}
