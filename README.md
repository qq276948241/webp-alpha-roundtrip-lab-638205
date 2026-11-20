webp
=====

```
██╗    ██╗███████╗██████╗ ██████╗
██║    ██║██╔════╝██╔══██╗██╔══██╗
██║ █╗ ██║█████╗  ██████╔╝██████╔╝
██║███╗██║██╔══╝  ██╔══██╗██╔═══╝
╚███╔███╔╝███████╗██████╔╝██║
 ╚══╝╚══╝ ╚══════╝╚═════╝ ╚═╝
```


[![Build Status](https://github.com/chai2010/webp/actions/workflows/test.yml/badge.svg)](https://github.com/chai2010/webp/actions/workflows/test.yml)
[![GoDoc](https://godoc.org/github.com/chai2010/webp?status.svg)](https://pkg.go.dev/github.com/chai2010/webp)
[![GitHub release](https://img.shields.io/github/v/tag/chai2010/webp.svg?label=release)](https://github.com/chai2010/webp/releases)
[![license](https://img.shields.io/github/license/chai2010/webp.svg)](https://github.com/chai2010/webp/blob/master/LICENSE)

Benchmark
=========

![](bench/benchmark_result.png)


Install
=======

This package bundles libwebp 1.4.0 and builds it with cgo, so a C toolchain
is required:

- **Linux**: `gcc` (e.g. `apt install build-essential`)
- **macOS**: Xcode Command Line Tools (`xcode-select --install`)
- **Windows**: `MinGW` ([download here](http://tdm-gcc.tdragon.net/download))

`CGO_ENABLED=1` must be set (it is the default when a C compiler is present).
Then run:

1. `go get github.com/chai2010/webp`
2. `go run hello.go`


Test
====

The test suite uses only offline fixtures from `./testdata` and the standard
`image` API; no network access or external codecs are needed:

```sh
go test ./...          # full suite
go test -race ./...    # with the race detector (concurrency isolation)
go test -run TestMetadata ./...   # metadata round-trip tests only
```

The suite covers: VP8/VP8L/VP8X+ALPH decoding, lossy/lossless/exact encoding,
per-pixel round-trips (alpha included), RIFF container validation (truncation,
odd-chunk padding, size overflow), ICCP/EXIF/XMP metadata round-trips and
chunk ordering, short-read/error readers, and concurrent use.

Container and metadata policy:

- The RIFF form size must fit the available data; truncation, chunk-size
  overflow and oversize canvases are rejected before any decode buffer is
  allocated. Trailing bytes past the declared RIFF size are ignored.
- Odd-size chunks are followed by one RIFF pad byte.
- `GetMetadata` returns the raw ICCP/EXIF/XMP chunk payload.
  `SetMetadata` replaces an existing chunk of the same format (last write
  wins), keeps the canonical chunk order (VP8X, ICCP, image, EXIF, XMP),
  and preserves unknown chunks. A missing format is an error, never a
  nil-error empty result.
- `DecodeGray` goes through the YUV colorspace (limited range); use
  `DecodeRGB`/`DecodeRGBA` when exact colors matter.


Example
=======

This is a simple example:

```Go
package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"

	"github.com/chai2010/webp"
)

func main() {
	var buf bytes.Buffer
	var width, height int
	var data []byte
	var err error

	// Load file data
	if data, err = ioutil.ReadFile("./testdata/1_webp_ll.webp"); err != nil {
		log.Println(err)
	}

	// GetInfo
	if width, height, _, err = webp.GetInfo(data); err != nil {
		log.Println(err)
	}
	fmt.Printf("width = %d, height = %d\n", width, height)

	// GetMetadata
	if metadata, err := webp.GetMetadata(data, "ICCP"); err != nil {
		fmt.Printf("Metadata: err = %v\n", err)
	} else {
		fmt.Printf("Metadata: %s\n", string(metadata))
	}

	// Decode webp
	m, err := webp.Decode(bytes.NewReader(data))
	if err != nil {
		log.Println(err)
	}

	// Encode lossless webp
	if err = webp.Encode(&buf, m, &webp.Options{Lossless: true}); err != nil {
		log.Println(err)
	}
	if err = ioutil.WriteFile("output.webp", buf.Bytes(), 0666); err != nil {
		log.Println(err)
	}
    
    fmt.Println("Save output.webp ok")
}
```

Decode and Encode as RGB format:

```Go
m, err := webp.DecodeRGB(data)
if err != nil {
	log.Fatal(err)
}

data, err := webp.EncodeRGB(m)
if err != nil {
	log.Fatal(err)
}
```

Give a Star! ⭐
===============

If you like project, please give it a star. Thanks!
