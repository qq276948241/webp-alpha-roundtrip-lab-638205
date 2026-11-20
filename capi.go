// Copyright 2014 <chaishushan{AT}gmail.com>. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//
// cgo pointer:
//
// Go1.3: Changes to the garbage collector
// http://golang.org/doc/go1.3#garbage_collector
//
// Go1.6:
// https://github.com/golang/proposal/blob/master/design/12416-cgo-pointers.md
//

package webp

/*
#cgo CFLAGS: -I./internal/libwebp-1.4.0/
#cgo CFLAGS: -I./internal/libwebp-1.4.0/src/
#cgo CFLAGS: -I./internal/include/
#cgo CFLAGS: -Wno-pointer-sign -DWEBP_USE_THREAD
#cgo !windows LDFLAGS: -lm

#include "webp.h"

#include <webp/decode.h>

#include <stdlib.h>
*/
import "C"
import (
	"errors"
	"fmt"
	"unsafe"
)

func webpGetInfo(data []byte) (width, height int, hasAlpha bool, err error) {
	if len(data) == 0 {
		err = errors.New("webpGetInfo: bad arguments, data is empty")
		return
	}
	if err = validateWebPHeaderPrefix(data); err != nil {
		return
	}
	if len(data) > maxWebpHeaderSize {
		data = data[:maxWebpHeaderSize]
	}

	var features C.WebPBitstreamFeatures
	if C.WebPGetFeatures((*C.uint8_t)(unsafe.Pointer(&data[0])), C.size_t(len(data)), &features) != C.VP8_STATUS_OK {
		err = errors.New("C.WebPGetFeatures: failed")
		return
	}
	width, height = int(features.width), int(features.height)
	hasAlpha = (features.has_alpha != 0)
	return
}

// webpDecodeSize validates the container and returns the decoded dimensions,
// rejecting oversize canvases before any decode buffer is allocated.
func webpDecodeSize(data []byte, channels int) (width, height int, err error) {
	if err = validateWebPContainer(data); err != nil {
		return
	}
	if width, height, _, err = webpGetInfo(data); err != nil {
		return
	}
	if err = checkDecodeBudget(width, height, channels); err != nil {
		return
	}
	return
}

func webpDecodeGray(data []byte) (pix []byte, width, height int, err error) {
	if len(data) == 0 {
		err = errors.New("webpDecodeGray: bad arguments")
		return
	}
	if width, height, err = webpDecodeSize(data, 1); err != nil {
		return
	}

	var cw, ch C.int
	var cptr = C.webpDecodeGray((*C.uint8_t)(unsafe.Pointer(&data[0])), C.size_t(len(data)), &cw, &ch)
	if cptr == nil {
		err = errors.New("webpDecodeGray: failed")
		return
	}
	defer C.free(unsafe.Pointer(cptr))

	pix = make([]byte, int(cw*ch*1))
	copy(pix, ((*[1 << 30]byte)(unsafe.Pointer(cptr)))[0:len(pix):len(pix)])
	width, height = int(cw), int(ch)
	return
}

func webpDecodeRGB(data []byte) (pix []byte, width, height int, err error) {
	if len(data) == 0 {
		err = errors.New("webpDecodeRGB: bad arguments")
		return
	}
	if width, height, err = webpDecodeSize(data, 3); err != nil {
		return
	}

	var cw, ch C.int
	var cptr = C.webpDecodeRGB((*C.uint8_t)(unsafe.Pointer(&data[0])), C.size_t(len(data)), &cw, &ch)
	if cptr == nil {
		err = errors.New("webpDecodeRGB: failed")
		return
	}
	defer C.free(unsafe.Pointer(cptr))

	pix = make([]byte, int(cw*ch*3))
	copy(pix, ((*[1 << 30]byte)(unsafe.Pointer(cptr)))[0:len(pix):len(pix)])
	width, height = int(cw), int(ch)
	return
}

func webpDecodeRGBA(data []byte) (pix []byte, width, height int, err error) {
	if len(data) == 0 {
		err = errors.New("webpDecodeRGBA: bad arguments")
		return
	}
	if width, height, err = webpDecodeSize(data, 4); err != nil {
		return
	}

	var cw, ch C.int
	var cptr = C.webpDecodeRGBA((*C.uint8_t)(unsafe.Pointer(&data[0])), C.size_t(len(data)), &cw, &ch)
	if cptr == nil {
		err = errors.New("webpDecodeRGBA: failed")
		return
	}
	defer C.free(unsafe.Pointer(cptr))

	pix = make([]byte, int(cw*ch*4))
	copy(pix, ((*[1 << 30]byte)(unsafe.Pointer(cptr)))[0:len(pix):len(pix)])
	width, height = int(cw), int(ch)
	return
}

// validScaleArgs validates DecodeXXXToSize arguments before the output
// buffer is allocated.
func validScaleArgs(who string, data []byte, width, height, channels int) error {
	if len(data) == 0 {
		return fmt.Errorf("%s: bad arguments, data is empty", who)
	}
	if err := checkDecodeBudget(width, height, channels); err != nil {
		return fmt.Errorf("%s: %v", who, err)
	}
	return validateWebPContainer(data)
}

func webpDecodeGrayToSize(data []byte, width, height int) (pix []byte, err error) {
	if err = validScaleArgs("webpDecodeGrayToSize", data, width, height, 1); err != nil {
		return
	}
	pix = make([]byte, int(width*height))
	stride := C.int(width)
	res := C.webpDecodeGrayToSize((*C.uint8_t)(unsafe.Pointer(&data[0])), C.size_t(len(data)), C.int(width), C.int(height), stride, (*C.uint8_t)(unsafe.Pointer(&pix[0])))
	if res != C.VP8_STATUS_OK {
		pix = nil
		err = errors.New("webpDecodeGrayToSize: failed")
	}
	return
}

func webpDecodeRGBToSize(data []byte, width, height int) (pix []byte, err error) {
	if err = validScaleArgs("webpDecodeRGBToSize", data, width, height, 3); err != nil {
		return
	}
	pix = make([]byte, int(3*width*height))
	stride := C.int(3 * width)
	res := C.webpDecodeRGBToSize((*C.uint8_t)(unsafe.Pointer(&data[0])), C.size_t(len(data)), C.int(width), C.int(height), stride, (*C.uint8_t)(unsafe.Pointer(&pix[0])))
	if res != C.VP8_STATUS_OK {
		pix = nil
		err = errors.New("webpDecodeRGBToSize: failed")
	}
	return
}

func webpDecodeRGBAToSize(data []byte, width, height int) (pix []byte, err error) {
	if err = validScaleArgs("webpDecodeRGBAToSize", data, width, height, 4); err != nil {
		return
	}
	pix = make([]byte, int(4*width*height))
	stride := C.int(4 * width)
	res := C.webpDecodeRGBAToSize((*C.uint8_t)(unsafe.Pointer(&data[0])), C.size_t(len(data)), C.int(width), C.int(height), stride, (*C.uint8_t)(unsafe.Pointer(&pix[0])))
	if res != C.VP8_STATUS_OK {
		pix = nil
		err = errors.New("webpDecodeRGBAToSize: failed")
	}
	return
}

func webpEncodeGray(pix []byte, width, height, stride int, quality float32) (output []byte, err error) {
	if err = validEncodeArgs("webpEncodeGray", pix, width, height, stride, 1); err != nil {
		return
	}
	if !validQuality(quality) {
		err = fmt.Errorf("webpEncodeGray: invalid quality %v", quality)
		return
	}

	var cptr_size C.size_t
	var cptr = C.webpEncodeGray(
		(*C.uint8_t)(unsafe.Pointer(&pix[0])), C.int(width), C.int(height),
		C.int(stride), C.float(quality),
		&cptr_size,
	)
	if cptr == nil || cptr_size == 0 {
		err = errors.New("webpEncodeGray: failed")
		return
	}
	defer C.free(unsafe.Pointer(cptr))

	output = make([]byte, int(cptr_size))
	copy(output, ((*[1 << 30]byte)(unsafe.Pointer(cptr)))[0:len(output):len(output)])
	return
}

func webpEncodeRGB(pix []byte, width, height, stride int, quality float32) (output []byte, err error) {
	if err = validEncodeArgs("webpEncodeRGB", pix, width, height, stride, 3); err != nil {
		return
	}
	if !validQuality(quality) {
		err = fmt.Errorf("webpEncodeRGB: invalid quality %v", quality)
		return
	}

	var cptr_size C.size_t
	var cptr = C.webpEncodeRGB(
		(*C.uint8_t)(unsafe.Pointer(&pix[0])), C.int(width), C.int(height),
		C.int(stride), C.float(quality),
		&cptr_size,
	)
	if cptr == nil || cptr_size == 0 {
		err = errors.New("webpEncodeRGB: failed")
		return
	}
	defer C.free(unsafe.Pointer(cptr))

	output = make([]byte, int(cptr_size))
	copy(output, ((*[1 << 30]byte)(unsafe.Pointer(cptr)))[0:len(output):len(output)])
	return
}

func webpEncodeRGBA(pix []byte, width, height, stride int, quality float32) (output []byte, err error) {
	if err = validEncodeArgs("webpEncodeRGBA", pix, width, height, stride, 4); err != nil {
		return
	}
	if !validQuality(quality) {
		err = fmt.Errorf("webpEncodeRGBA: invalid quality %v", quality)
		return
	}

	var cptr_size C.size_t
	var cptr = C.webpEncodeRGBA(
		(*C.uint8_t)(unsafe.Pointer(&pix[0])), C.int(width), C.int(height),
		C.int(stride), C.float(quality),
		&cptr_size,
	)
	if cptr == nil || cptr_size == 0 {
		err = errors.New("webpEncodeRGBA: failed")
		return
	}
	defer C.free(unsafe.Pointer(cptr))

	output = make([]byte, int(cptr_size))
	copy(output, ((*[1 << 30]byte)(unsafe.Pointer(cptr)))[0:len(output):len(output)])
	return
}

func webpEncodeLosslessGray(pix []byte, width, height, stride int) (output []byte, err error) {
	if err = validEncodeArgs("webpEncodeLosslessGray", pix, width, height, stride, 1); err != nil {
		return
	}

	var cptr_size C.size_t
	var cptr = C.webpEncodeLosslessGray(
		(*C.uint8_t)(unsafe.Pointer(&pix[0])), C.int(width), C.int(height),
		C.int(stride),
		&cptr_size,
	)
	if cptr == nil || cptr_size == 0 {
		err = errors.New("webpEncodeLosslessGray: failed")
		return
	}
	defer C.free(unsafe.Pointer(cptr))

	output = make([]byte, int(cptr_size))
	copy(output, ((*[1 << 30]byte)(unsafe.Pointer(cptr)))[0:len(output):len(output)])
	return
}

func webpEncodeLosslessRGB(pix []byte, width, height, stride int) (output []byte, err error) {
	if err = validEncodeArgs("webpEncodeLosslessRGB", pix, width, height, stride, 3); err != nil {
		return
	}

	var cptr_size C.size_t
	var cptr = C.webpEncodeLosslessRGB(
		(*C.uint8_t)(unsafe.Pointer(&pix[0])), C.int(width), C.int(height),
		C.int(stride),
		&cptr_size,
	)
	if cptr == nil || cptr_size == 0 {
		err = errors.New("webpEncodeLosslessRGB: failed")
		return
	}
	defer C.free(unsafe.Pointer(cptr))

	output = make([]byte, int(cptr_size))
	copy(output, ((*[1 << 30]byte)(unsafe.Pointer(cptr)))[0:len(output):len(output)])
	return
}

func webpEncodeLosslessRGBA(exact int, pix []byte, width, height, stride int) (output []byte, err error) {
	if err = validEncodeArgs("webpEncodeLosslessRGBA", pix, width, height, stride, 4); err != nil {
		return
	}

	var cptr_size C.size_t
	var cptr = C.webpEncodeLosslessRGBA(
		C.int(exact), (*C.uint8_t)(unsafe.Pointer(&pix[0])), C.int(width), C.int(height),
		C.int(stride),
		&cptr_size,
	)
	if cptr == nil || cptr_size == 0 {
		err = errors.New("webpEncodeLosslessRGBA: failed")
		return
	}
	defer C.free(unsafe.Pointer(cptr))

	output = make([]byte, int(cptr_size))
	copy(output, ((*[1 << 30]byte)(unsafe.Pointer(cptr)))[0:len(output):len(output)])
	return
}

func webpGetEXIF(data []byte) (metadata []byte, err error) {
	if len(data) == 0 {
		err = errors.New("webpGetEXIF: bad arguments")
		return
	}

	var cptr_size C.size_t
	var cptr = C.webpGetEXIF(
		(*C.uint8_t)(unsafe.Pointer(&data[0])), C.size_t(len(data)),
		&cptr_size,
	)
	if cptr == nil || cptr_size == 0 {
		err = errors.New("webpGetEXIF: failed")
		return
	}
	defer C.free(unsafe.Pointer(cptr))

	metadata = make([]byte, int(cptr_size))
	copy(metadata, ((*[1 << 30]byte)(unsafe.Pointer(cptr)))[0:len(metadata):len(metadata)])
	return
}
func webpGetICCP(data []byte) (metadata []byte, err error) {
	if len(data) == 0 {
		err = errors.New("webpGetICCP: bad arguments")
		return
	}

	var cptr_size C.size_t
	var cptr = C.webpGetICCP(
		(*C.uint8_t)(unsafe.Pointer(&data[0])), C.size_t(len(data)),
		&cptr_size,
	)
	if cptr == nil || cptr_size == 0 {
		err = errors.New("webpGetICCP: failed")
		return
	}
	defer C.free(unsafe.Pointer(cptr))

	metadata = make([]byte, int(cptr_size))
	copy(metadata, ((*[1 << 30]byte)(unsafe.Pointer(cptr)))[0:len(metadata):len(metadata)])
	return
}
func webpGetXMP(data []byte) (metadata []byte, err error) {
	if len(data) == 0 {
		err = errors.New("webpGetXMP: bad arguments")
		return
	}

	var cptr_size C.size_t
	var cptr = C.webpGetXMP(
		(*C.uint8_t)(unsafe.Pointer(&data[0])), C.size_t(len(data)),
		&cptr_size,
	)
	if cptr == nil || cptr_size == 0 {
		err = errors.New("webpGetXMP: failed")
		return
	}
	defer C.free(unsafe.Pointer(cptr))

	metadata = make([]byte, int(cptr_size))
	copy(metadata, ((*[1 << 30]byte)(unsafe.Pointer(cptr)))[0:len(metadata):len(metadata)])
	return
}
func webpGetMetadata(data []byte, format string) (metadata []byte, err error) {
	if len(data) == 0 {
		err = errors.New("webpGetMetadata: bad arguments")
		return
	}
	if err = validateWebPContainer(data); err != nil {
		return
	}

	switch format {
	case "EXIF":
		return webpGetEXIF(data)
	case "ICCP":
		return webpGetICCP(data)
	case "XMP":
		return webpGetXMP(data)
	default:
		err = errors.New("webpGetMetadata: unknown format")
		return
	}
}

func webpSetEXIF(data, metadata []byte) (newData []byte, err error) {
	if len(data) == 0 || len(metadata) == 0 {
		err = errors.New("webpSetEXIF: bad arguments")
		return
	}

	var cptr_size C.size_t
	var cptr = C.webpSetEXIF(
		(*C.uint8_t)(unsafe.Pointer(&data[0])), C.size_t(len(data)),
		(*C.char)(unsafe.Pointer(&metadata[0])), C.size_t(len(metadata)),
		&cptr_size,
	)
	if cptr == nil || cptr_size == 0 {
		err = errors.New("webpSetEXIF: failed")
		return
	}
	defer C.free(unsafe.Pointer(cptr))

	newData = make([]byte, int(cptr_size))
	copy(newData, ((*[1 << 30]byte)(unsafe.Pointer(cptr)))[0:len(newData):len(newData)])
	return
}
func webpSetICCP(data, metadata []byte) (newData []byte, err error) {
	if len(data) == 0 || len(metadata) == 0 {
		err = errors.New("webpSetICCP: bad arguments")
		return
	}

	var cptr_size C.size_t
	var cptr = C.webpSetICCP(
		(*C.uint8_t)(unsafe.Pointer(&data[0])), C.size_t(len(data)),
		(*C.char)(unsafe.Pointer(&metadata[0])), C.size_t(len(metadata)),
		&cptr_size,
	)
	if cptr == nil || cptr_size == 0 {
		err = errors.New("webpSetICCP: failed")
		return
	}
	defer C.free(unsafe.Pointer(cptr))

	newData = make([]byte, int(cptr_size))
	copy(newData, ((*[1 << 30]byte)(unsafe.Pointer(cptr)))[0:len(newData):len(newData)])
	return
}
func webpSetXMP(data, metadata []byte) (newData []byte, err error) {
	if len(data) == 0 || len(metadata) == 0 {
		err = errors.New("webpSetXMP: bad arguments")
		return
	}

	var cptr_size C.size_t
	var cptr = C.webpSetXMP(
		(*C.uint8_t)(unsafe.Pointer(&data[0])), C.size_t(len(data)),
		(*C.char)(unsafe.Pointer(&metadata[0])), C.size_t(len(metadata)),
		&cptr_size,
	)
	if cptr == nil || cptr_size == 0 {
		err = errors.New("webpSetXMP: failed")
		return
	}
	defer C.free(unsafe.Pointer(cptr))

	newData = make([]byte, int(cptr_size))
	copy(newData, ((*[1 << 30]byte)(unsafe.Pointer(cptr)))[0:len(newData):len(newData)])
	return
}
func webpSetMetadata(data, metadata []byte, format string) (newData []byte, err error) {
	if len(data) == 0 || len(metadata) == 0 {
		err = errors.New("webpSetMetadata: bad arguments")
		return
	}
	if err = validateWebPContainer(data); err != nil {
		return
	}

	switch format {
	case "EXIF":
		return webpSetEXIF(data, metadata)
	case "ICCP":
		return webpSetICCP(data, metadata)
	case "XMP":
		return webpSetXMP(data, metadata)
	default:
		err = errors.New("webpSetMetadata: unknown format")
		return
	}
}

func webpDelEXIF(data []byte) (newData []byte, err error) {
	if len(data) == 0 {
		err = errors.New("webpDelEXIF: bad arguments")
		return
	}

	var cptr_size C.size_t
	var cptr = C.webpDelEXIF(
		(*C.uint8_t)(unsafe.Pointer(&data[0])), C.size_t(len(data)),
		&cptr_size,
	)
	if cptr == nil || cptr_size == 0 {
		err = errors.New("webpDelEXIF: failed")
		return
	}
	defer C.free(unsafe.Pointer(cptr))

	newData = make([]byte, int(cptr_size))
	copy(newData, ((*[1 << 30]byte)(unsafe.Pointer(cptr)))[0:len(newData):len(newData)])
	return
}
func webpDelICCP(data []byte) (newData []byte, err error) {
	if len(data) == 0 {
		err = errors.New("webpDelICCP: bad arguments")
		return
	}

	var cptr_size C.size_t
	var cptr = C.webpDelICCP(
		(*C.uint8_t)(unsafe.Pointer(&data[0])), C.size_t(len(data)),
		&cptr_size,
	)
	if cptr == nil || cptr_size == 0 {
		err = errors.New("webpDelICCP: failed")
		return
	}
	defer C.free(unsafe.Pointer(cptr))

	newData = make([]byte, int(cptr_size))
	copy(newData, ((*[1 << 30]byte)(unsafe.Pointer(cptr)))[0:len(newData):len(newData)])
	return
}
func webpDelXMP(data []byte) (newData []byte, err error) {
	if len(data) == 0 {
		err = errors.New("webpDelXMP: bad arguments")
		return
	}

	var cptr_size C.size_t
	var cptr = C.webpDelXMP(
		(*C.uint8_t)(unsafe.Pointer(&data[0])), C.size_t(len(data)),
		&cptr_size,
	)
	if cptr == nil || cptr_size == 0 {
		err = errors.New("webpDelXMP: failed")
		return
	}
	defer C.free(unsafe.Pointer(cptr))

	newData = make([]byte, int(cptr_size))
	copy(newData, ((*[1 << 30]byte)(unsafe.Pointer(cptr)))[0:len(newData):len(newData)])
	return
}
