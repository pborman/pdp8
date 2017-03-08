// Copyright 2017 Paul Borman
// Use of this source code is governed by a Apache-style
// license found in the LICENSE file.  It also can be found at
// https://github.com/pborman/pdp8/blob/master/LICENSE

// Program 8rm is an experimental program to remove files from a PDP-8
// disk image.
//
// The following examples of path names assume PDP8_IMAGE is /tmp/os8.rk05:
//
//  PATH                   DRIVE         SIDE FILE
//  foobar.xy               /tmp/os8.rk05  A  FOOBAR.XY
//  b:foobar.xy             /tmp/os8.rk05  B  FOOBAR.XY
//  ./os8.rk05/foobar.xy    ./os8.rk05     A  FOOBAR.XY
//  ./os8.rk05/a:foobar.xy  ./os8.rk05     A  FOOBAR.XY
//  ./os8.rk05/b:foobar.xy  ./os8.rk05     B  FOOBAR.XY
package main

import (
	"fmt"
	"os"
	"strings"

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

func main() {
	var path string
	switch len(os.Args) {
	case 2:
		path = os.Args[1]
	default:
		exit("usage: 8rm [IMAGE/]FILE")
	}
	image := os8fs.DefaultImage
	if x := strings.LastIndex(path, "/"); x >= 0 {
		image = path[:x]
		path = path[x+1:]
	}

	d, err := os8fs.OpenImage(image, true)
	if err != nil {
		exit(err)
	}
	if err := d.Remove(path); err != nil {
		exit(err)
	}
}
