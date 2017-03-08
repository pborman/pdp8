// Copyright 2017 Paul Borman
// Use of this source code is governed by a Apache-style
// license found in the LICENSE file.  It also can be found at
// https://github.com/pborman/pdp8/blob/master/LICENSE

// Program 8dir displays the directory listing of a PDP-8 disk image.  If the
// path to the image is not provided, environment variable PDP8_IMAGE is used.
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
	case 1:
		path = os.Getenv("PDP8_IMAGE")
		if path == "" {
			exit("usage: 8dir IMAGE")
		}
	case 2:
		path = os.Args[1]
	default:
		exit("usage: 8dir [IMAGE]")
	}
	d, err := os8fs.OpenImage(path, false)
	if err != nil {
		exit(err)
	}
	fis, err := d.List()
	for _, fi := range fis {
		date := fi.Date.String()
		if date != "" {
			date = " " + date
		}
		fmt.Printf("%-11s %-3d%s\n", fi.Name, fi.Size, date)
	}
	if err != nil {
		exit(err)
	}
}
