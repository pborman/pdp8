// Copyright 2017 Paul Borman
// Use of this source code is governed by a Apache-style
// license found in the LICENSE file.  It also can be found at
// https://github.com/pborman/pdp8/blob/master/LICENSE

// Program 8cat is used to display files from a PDP-8 disk image.
//
//   Usage: 8cat [-678r] [IMAGE/]FILE
//    -6    decode as 6 bit ascii
//    -7    decode as 7 bit ascii
//    -8    decode as packed 8 bit bytes
//    -r    raw bytes
//
// By default, 8cat tries to determine if the file is encoded as ASCII6,
// 7 bit ASCII, or is a binary file.
//
// The disk image is either specified as the directory component of the file to
// cat, or by the environment variable PDP_IMAGE.
//
// Assuming PDP8_IMAGE is set to /tmp/os8.rk05:
//
//  PATH                   DRIVE         SIDE FILE
//  foobar.xy               /tmp/os8.rk05  A   FOOBAR.XY
//  b:foobar.xy             /tmp/os8.rk05  B   FOOBAR.XY
//  ./os8.rk05/foobar.xy    ./os8.rk05     A   FOOBAR.XY
//  ./os8.rk05/a:foobar.xy  ./os8.rk05     A   FOOBAR.XY
//  ./os8.rk05/b:foobar.xy  ./os8.rk05     B   FOOBAR.XY
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/pborman/getopt"
	"github.com/pborman/pdp8/os8fs"
)

func exit(v ...interface{}) {
	fmt.Fprintln(os.Stderr, v...)
	os.Exit(1)
}
func exitf(format string, v ...interface{}) {
	if !strings.HasSuffix(format, "\n") {
		format += "\n"
	}
	fmt.Fprintf(os.Stderr, format, v...)
	os.Exit(1)
}

func isAscii(data []byte) bool {
	bad := 0
	for _, c := range data {
		if c >= ' ' && c != 0177 {
			continue
		}
		switch c {
		case '\f', '\r', '\t', '\n':
		default:
			bad++
		}
	}
	return bad*16 < len(data)
}

func isAscii6(data []byte) bool {
	bad := 0
	for _, c := range data {
		if c == '@' {
			bad++
		}
	}
	return bad*16 < len(data)
}

func main() {
	getopt.SetParameters("[IMAGE/]FILE")
	as6 := getopt.Bool('6', "decode as 6 bit ascii")
	as7 := getopt.Bool('7', "decode as 7 bit ascii")
	as8 := getopt.Bool('8', "decode as packed 8 bit bytes")
	raw := getopt.Bool('r', "raw bytes")
	getopt.Parse()
	args := getopt.Args()
	if len(args) != 1 {
		getopt.PrintUsage(os.Stderr)
		os.Exit(1)
	}
	f, err := os8fs.GetFile(args[0])
	if err != nil {
		exit(err)
	}
	switch {
	case *as6:
		_, err = os.Stdout.Write(f.ASCII6())
	case *as7:
		_, err = os.Stdout.Write(f.ASCII(true))
	case *as8:
		_, err = os.Stdout.Write(f.ASCII(false))
	case *raw:
		_, err = os.Stdout.Write(f.Bytes())
	default:
		if bytes := f.ASCII(true); isAscii(bytes) {
			_, err = os.Stdout.Write(bytes)
		} else if bytes = f.ASCII6(); isAscii6(bytes) {
			_, err = os.Stdout.Write(bytes)
		} else {
			_, err = os.Stdout.Write(f.Bytes())
		}
	}
	if err != nil {
		exit("%s: %v", args[1], err)
	}
}
