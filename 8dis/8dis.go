// Copyright 2017 Paul Borman
// Use of this source code is governed by a Apache-style
// license found in the LICENSE file.  It also can be found at
// https://github.com/pborman/pdp8/blob/master/LICENSE

// Program 8dis is an experimental disassembler for the PDP-8.  It is more of
// a toy at this point.  If the named file ends with .BN then it is decoded
// as a BIN file.  Other files currently are just treated as raw instructions.
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
	w := bufio.NewWriter(os.Stdout)
	if strings.HasSuffix(f.Name(), ".BN") {
		start, mem := readBin(f.ASCII(false))
		for i, word := range mem {
			if word != 0 {
				fmt.Fprintf(w, "%04o: %04o %-30s %2s\n", start+i, word, decode(uint16(start+i), word), os8fs.ASCII6(word))
			}
		}
	} else {
		return
		words := f.Words()

		for i, word := range words {
			fmt.Fprintf(w, "%04o: %04o %-30s %2s\n", i, word, decode(uint16(i), word), os8fs.ASCII6(word))
		}
	}
	w.Flush()
}

var ops = []string{"AND", "TAD", "ISZ", "DCA", "JMS", "JMP", "IOT", "OPR"}

func decode(a, w uint16) string {
	if op, ok := fixed[w]; ok {
		return op
	}
	op := (w >> 9) & 7

	if op < 6 {
		addr := w & 0177
		if w&0200 != 0 {
			addr |= a & 07600
		}
		if w&0400 != 0 {
			return fmt.Sprintf("%s [%04o]", ops[op], addr)
		}
		return fmt.Sprintf("%s %04o", ops[op], addr)
	}
	switch {
	case w&07000 == 06000:
		return decodeiot(a, w)
	case w&07400 == 07000:
		return decode1(a, w)
	case w&07401 == 07400:
		return decode2(a, w)
	case w&07401 == 07401:
		return decodemq(a, w)
	default:
		return fmt.Sprintf("%04o", w)
	}
}

func decode1(a, w uint16) string {
	var parts []string
	if w == 7000 {
		return "NOP"
	}
	if w&0200 != 0 {
		parts = append(parts, "CLA")
	}
	if w&0100 != 0 {
		parts = append(parts, "CLL")
	}
	if w&0040 != 0 {
		parts = append(parts, "CMA")
	}
	if w&0020 != 0 {
		parts = append(parts, "CML")
	}
	if w&0001 != 0 {
		parts = append(parts, "IAC")
	}
	switch w & 0016 {
	case 0000:
	case 0002:
		parts = append(parts, "BSW")
	case 0010:
		parts = append(parts, "RAR")
	case 0012:
		parts = append(parts, "RTR")
	case 0004:
		parts = append(parts, "RAL")
	case 0006:
		parts = append(parts, "RTL")
	case 0014:
		parts = append(parts, "RARL?")
	case 0016:
		parts = append(parts, "RTRL?")
	}
	return strings.Join(parts, " ")
}

func decode2(a, w uint16) string {
	var parts []string
	if w == 7400 || w == 7410 {
		return "NOP"
	}
	jmps := []string{"SMA", "SZA", "SNL"}
	if w&010 != 0 {
		jmps = []string{"SPA", "SNA", "SZL"}
	}
	if w&0100 != 0 {
		parts = append(parts, jmps[0])
	}
	if w&0040 != 0 {
		parts = append(parts, jmps[1])
	}
	if w&0020 != 0 {
		parts = append(parts, jmps[2])
	}
	if w&0200 != 0 {
		parts = append(parts, "CLA")
	}
	if w&0004 != 0 {
		parts = append(parts, "OSR")
	}
	if w&0002 != 0 {
		parts = append(parts, "HLT")
	}
	return strings.Join(parts, " ")
}

func decodemq(a, w uint16) string {
	if w == 7401 {
		return "NOP"
	}
	A := decodeA(a, w)
	B := decodeB(a, w)
	if A == B {
		return A
	}
	return A + "/" + "B"
}

func decodeA(a, w uint16) string {
	var parts []string
	if w&0200 != 0 {
		parts = append(parts, "CLA")
	}
	if w&0100 != 0 {
		parts = append(parts, "MQA")
	}
	if w&0040 != 0 {
		parts = append(parts, "SCA")
	}
	if w&0020 != 0 {
		parts = append(parts, "MQL")
	}
	op := (w >> 1) & 7
	if op == 4 {
		if len(parts) > 0 {
			return fmt.Sprintf("%04o", w)
		}
		return "NMI"
	}
	ops := []string{"NOP", "SCL", "MUY", "DVI", "NMI", "SHL", "ASR", "LSR"}
	if op > 0 {
		parts = append(parts, ops[op])
	}
	return strings.Join(parts, " ")
}

var specialB = map[uint16]string{
	07763: "DLD",
	07445: "DST",
	07443: "DAD",
	07573: "DPIC",
	07575: "DCM",
	07451: "DPSZ",
}

func decodeB(a, w uint16) string {
	if op, ok := specialB[w]; ok {
		return op
	}
	var parts []string
	op := ((w >> 1) & 7) | ((w & 0040) >> 2)
	ops := []string{"NOP", "ACS", "MUY", "DVI", "NMI", "SHL", "ASR", "LSR", "SCA", "DAD", "DST", "SWBA", "DPSZ", "DPIC", "DCM", "SAM"}
	if op == 4 {
		if len(parts) > 0 {
			return fmt.Sprintf("%04o", w)
		}
		return "NMI"
	}
	if op == 015 || op == 016 {
		if w&0120 != 0 {
			return fmt.Sprintf("%04o", w)
		}
	}
	if w&0200 != 0 {
		parts = append(parts, "CLA")
	}
	if w&0100 != 0 {
		parts = append(parts, "MBA")
	}
	if w&0020 != 0 {
		parts = append(parts, "MQL")
	}
	if op > 0 {
		parts = append(parts, ops[op])
	}
	return strings.Join(parts, " ")
}

func decodeiot(a, w uint16) string {
	dev := w >> 3 & 077
	iot := w & 07
	return fmt.Sprintf("IOT DEV%02o %o", dev, iot)
}

var fixed = map[uint16]string{
	06000: "SKON",
	06001: "ION",
	06002: "IOF",
	06003: "SRQ",
	06004: "GTF",
	06005: "RTF",
	06006: "SGT",
	06007: "CAF",
	06010: "RPE",
	06011: "RSF",
	06012: "RRB",
	06014: "RCF",
	06016: "RCC",
	06020: "PCE",
	06021: "PSF",
	06022: "PCF",
	06024: "PPC",
	06026: "PLS",
	06030: "KCF",
	06031: "KSF",
	06032: "KCC",
	06034: "KRS",
	06035: "KIE",
	06036: "KRB",
	06040: "SPF",
	06041: "TSF",
	06042: "TCF",
	06044: "TPC",
	06045: "SPI",
	06046: "TLS",
	06601: "DCMA",
	06603: "DMAR",
	06605: "DMAW",
	06611: "DCEA",
	06612: "DSAC",
	06615: "DEAL",
	06616: "DEAC",
	06621: "DFSE",
	06622: "DFSC",
	06626: "DMAC",
	06761: "DTRA",
	06762: "DTCA",
	06764: "DTXA",
	06771: "DTSF",
	06772: "DTRB",
	06774: "DTLB",
	07403: "SCL/ASC",
	07431: "SWAB",
	07447: "SWBA",
	07457: "SAM",
}

func skipHeader(data []byte) []byte {
	for i, c := range data {
		if c&0x80 == 0 {
			fmt.Printf("skipped %d\n", i)
			return data[i:]
		}
	}
	return nil
}

type block struct {
	start int
	words []uint16
}

func readBin(data []byte) (int, []uint16) {
	data = skipHeader(data)
	if len(data) == 0 {
		return 0, nil
	}
	var addr uint16
	var blocks []block
	var b block
	for i := 0; i+1 < len(data); i += 2 {
		if data[i]&0200 != 0 {
			break
		}
		if data[i]&0100 != 0 {
			addr = (uint16(data[i]&077) << 6) | uint16(data[i+1]&077)
			if len(b.words) > 0 {
				blocks = append(blocks, b)
			}
			b.start = int(addr)
			b.words = nil
			continue
		}
		value := (uint16(data[i]&077) << 6) | uint16(data[i+1]&077)
		b.words = append(b.words, value)
	}
	if len(b.words) > 0 {
		blocks = append(blocks, b)
	}
	if len(blocks) == 0 {
		return 0, nil
	}
	lb := blocks[len(blocks)-1]
	if len(lb.words) > 0 {
		lb.words = lb.words[:len(lb.words)-1]
		blocks[len(blocks)-1] = lb
	}
	start := blocks[0].start
	stop := blocks[0].start + len(blocks[0].words)
	for _, b := range blocks[1:] {
		t := b.start
		p := b.start + len(b.words)
		if t < start {
			start = t
		}
		if p > stop {
			stop = p
		}
	}
	mem := make([]uint16, stop-start)
	for _, b := range blocks {
		copy(mem[b.start-start:], b.words)
	}
	return start, mem
}
