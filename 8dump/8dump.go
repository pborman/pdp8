// Copyright 2017 Paul Borman
// Use of this source code is governed by a Apache-style
// license found in the LICENSE file.  It also can be found at
// https://github.com/pborman/pdp8/blob/master/LICENSE

// Program 8dump dumps the named file in octal, ASCII6 and ASCII.
//
//   Usage: 8dump [-67o] [IMAGE/]FILE
//    -6    dump 6 bit ascii
//    -7    dump 7 bit ascii
//    -o    dump octal
//
// If no options are provided, octal and ASCII6 are displayed, otherwise,
// octal is displayed and ASCII6 (-6 provided) and ASCII (-7 provided).  The
// -o option is to enable displaying of only octal and no ASCII.
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
	"bufio"
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

func main() {
	getopt.SetParameters("[IMAGE/]FILE")
	a6 := getopt.Bool('6', "dump 6 bit ascii")
	a7 := getopt.Bool('7', "dump 7 bit ascii")
	n := getopt.Bool('o', "dump octal")
	getopt.Parse()
	args := getopt.Args()

	if !*n && !*a7 && !*a6 {
		*a6 = true
	}

	if len(args) != 1 {
		getopt.PrintUsage(os.Stderr)
		os.Exit(1)
	}
	file, err := os8fs.GetFile(args[0])
	if err != nil {
		exit(err)
	}
	words := file.Words()
	w := bufio.NewWriter(os.Stdout)
	for i := 0; i < len(words); i += 8 {
		fmt.Fprintf(w, "%07o:", i)
		for _, word := range words[i : i+4] {
			fmt.Fprintf(w, " %04o", word)
		}
		fmt.Fprintf(w, " ")
		for _, word := range words[i+4 : i+8] {
			fmt.Fprintf(w, " %04o", word)
		}
		if *a6 {
			fmt.Fprintf(w, "  ")
			for _, word := range words[i : i+4] {
				fmt.Fprintf(w, "%s", os8fs.ASCII6(word))
			}
			fmt.Fprintf(w, " ")
			for _, word := range words[i+4 : i+8] {
				fmt.Fprintf(w, "%s", os8fs.ASCII6(word))
			}
		}
		if *a7 {
			fmt.Fprintf(w, "  ")
			var b [3]byte
			os8fs.ASCII8(b[:], words[i:], 0x7f)
			fmt.Fprintf(w, "%s", fix(b))
			os8fs.ASCII8(b[:], words[i+2:], 0x7f)
			fmt.Fprintf(w, "%s", fix(b))
			os8fs.ASCII8(b[:], words[i+4:], 0x7f)
			fmt.Fprintf(w, " %s", fix(b))
			os8fs.ASCII8(b[:], words[i+5:], 0x7f)
			fmt.Fprintf(w, "%s", fix(b))
		}
		fmt.Fprintln(w)
	}
	w.Flush()
}

func fix(b [3]byte)[3]byte {
	for i, c := range b {
		if c < ' ' || c > '~' {
			b[i] = '.'
		}
	}
	return b
}
