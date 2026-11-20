// Copyright 2026 <chaishushan{AT}gmail.com>. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webp

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// This file implements a shared, pure-Go validation layer for the RIFF/WEBP
// container. Decode, DecodeRGB, DecodeRGBA, DecodeGray, GetInfo and
// GetMetadata all funnel through these helpers so that a given byte stream
// yields the same dimensions, the same pixel semantics and the same errors
// from every entry point.
//
// All length arithmetic is done in int64 and every chunk bound is checked
// before any decode buffer is allocated, so truncated streams, odd-size
// chunk padding and 32-bit length overflows are rejected up front.
//
// Container policy (also documented in the package doc):
//
//   - The RIFF form size must not exceed the available data (truncation is
//     an error). Trailing bytes beyond the declared RIFF size are ignored.
//   - The first chunk must be VP8 , VP8L or VP8X.
//   - A VP8X chunk must be at least 10 bytes and must be followed by an
//     image chunk (VP8 /VP8L) or an animation (ANIM) chunk.
//   - Odd-size chunks are followed by exactly one padding byte, as required
//     by the RIFF spec.
//   - Unknown chunks are skipped on decode and preserved by SetMetadata;
//     duplicate ICCP/EXIF/XMP chunks are replaced by SetMetadata (last one
//     wins), while GetMetadata returns the first one, matching libwebp demux.

const (
	// maxWebpDimension is the largest width/height the WebP bitstream and
	// the libwebp encoder support (WEBP_MAX_DIMENSION).
	maxWebpDimension = 16383

	// maxDecodeBufferSize caps the decoded pixel buffer (channels included)
	// that the Decode* family will allocate. Streams advertising a canvas
	// that would exceed this budget are rejected before any allocation.
	maxDecodeBufferSize = 1 << 30 // 1 GiB
)

var (
	errInvalidRIFFHeader = errors.New("webp: invalid RIFF/WEBP header")
	errTruncatedRIFF     = errors.New("webp: truncated RIFF data")
	errChunkSizeOverflow = errors.New("webp: chunk size overflow")
	errMissingImageChunk = errors.New("webp: missing VP8/VP8L image chunk")
	errDimensionOverflow = errors.New("webp: image dimensions exceed decode budget")
)

// riffChunk describes one chunk inside a RIFF/WEBP file. Payload is a
// subslice of the input data (padding byte excluded).
type riffChunk struct {
	FourCC  string
	Payload []byte
}

// validateWebPHeaderPrefix performs the cheap structural checks that are
// possible on a (possibly truncated) prefix of a WebP file: RIFF/WEBP magic
// and a plausible first chunk. GetInfo and DecodeConfig use this because
// they only read the header; full validation happens in parseWebPContainer.
func validateWebPHeaderPrefix(data []byte) error {
	if len(data) < 12 {
		return errInvalidRIFFHeader
	}
	if string(data[0:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		return errInvalidRIFFHeader
	}
	riffSize := int64(binary.LittleEndian.Uint32(data[4:8]))
	if riffSize < 4 { // must at least hold the "WEBP" form type
		return errInvalidRIFFHeader
	}
	if len(data) >= 16 {
		switch string(data[12:16]) {
		case "VP8 ", "VP8L", "VP8X":
		default:
			return fmt.Errorf("webp: unexpected first chunk %q", data[12:16])
		}
	}
	return nil
}

// parseWebPContainer fully validates a complete WebP byte stream and returns
// its chunk list in file order. It never allocates proportionally to the
// declared sizes: bounds are checked with overflow-safe arithmetic before
// any slice is taken.
func parseWebPContainer(data []byte) ([]riffChunk, error) {
	if err := validateWebPHeaderPrefix(data); err != nil {
		return nil, err
	}
	riffSize := int64(binary.LittleEndian.Uint32(data[4:8]))
	declared := 8 + riffSize // RIFF size excludes its own 8-byte header
	if declared > int64(len(data)) {
		return nil, errTruncatedRIFF
	}
	end := int(declared) // fits: declared <= len(data)

	var chunks []riffChunk
	hasImage, hasAnim := false, false
	for pos := 12; pos < end; {
		if end-pos < 8 {
			return nil, fmt.Errorf("webp: truncated chunk header at offset %d", pos)
		}
		fourcc := string(data[pos : pos+4])
		size := int64(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		payload := int64(pos) + 8
		if payload+size > int64(end) {
			return nil, errChunkSizeOverflow
		}
		chunks = append(chunks, riffChunk{
			FourCC:  fourcc,
			Payload: data[payload : payload+size],
		})
		switch fourcc {
		case "VP8 ", "VP8L":
			hasImage = true
		case "ANIM":
			hasAnim = true
		}
		// Advance past the payload plus the RIFF pad byte for odd sizes.
		// Tolerate a final odd-size chunk whose pad byte is not counted in
		// the declared RIFF size (a common encoder quirk).
		next := payload + size + (size & 1)
		if next > int64(end) {
			if payload+size == int64(end) {
				break
			}
			return nil, errChunkSizeOverflow
		}
		pos = int(next)
	}
	if len(chunks) == 0 {
		return nil, errMissingImageChunk
	}
	switch chunks[0].FourCC {
	case "VP8 ", "VP8L":
		// Simple (non-extended) file: the image chunk is the payload.
	case "VP8X":
		if len(chunks[0].Payload) < 10 {
			return nil, fmt.Errorf("webp: VP8X chunk too small (%d bytes)", len(chunks[0].Payload))
		}
		if !hasImage && !hasAnim {
			return nil, errMissingImageChunk
		}
	default:
		return nil, fmt.Errorf("webp: unexpected first chunk %q", chunks[0].FourCC)
	}
	return chunks, nil
}

// validateWebPContainer validates a complete WebP byte stream prior to
// decoding. It is called by every decode entry point so corrupt containers
// fail identically everywhere, before any decode buffer is allocated.
func validateWebPContainer(data []byte) error {
	_, err := parseWebPContainer(data)
	return err
}

// checkDecodeBudget rejects images whose decoded pixel buffer would exceed
// maxDecodeBufferSize, before the buffer is allocated. channels is the
// number of bytes per pixel of the requested output format.
func checkDecodeBudget(width, height, channels int) error {
	if width <= 0 || height <= 0 {
		return fmt.Errorf("webp: invalid dimensions %dx%d", width, height)
	}
	if int64(width)*int64(height)*int64(channels) > maxDecodeBufferSize {
		return errDimensionOverflow
	}
	return nil
}

// validEncodeArgs validates the common encode preconditions: dimensions
// within the codec limit, stride large enough for one row, and a pixel
// slice that actually holds the last row. All arithmetic is overflow-safe.
func validEncodeArgs(who string, pix []byte, width, height, stride, channels int) error {
	if width <= 0 || height <= 0 {
		return fmt.Errorf("%s: bad dimensions %dx%d", who, width, height)
	}
	if width > maxWebpDimension || height > maxWebpDimension {
		return fmt.Errorf("%s: dimensions %dx%d exceed %d", who, width, height, maxWebpDimension)
	}
	if stride < width*channels {
		return fmt.Errorf("%s: stride %d too small for width %d", who, stride, width)
	}
	need := int64(height-1)*int64(stride) + int64(width)*int64(channels)
	if int64(len(pix)) < need {
		return fmt.Errorf("%s: pixel buffer too small: have %d, need %d", who, len(pix), need)
	}
	return nil
}

// validQuality reports whether a lossy quality factor is in [0, 100].
func validQuality(quality float32) bool {
	return quality >= 0 && quality <= 100
}
