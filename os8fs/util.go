// Copyright 2017 Paul Borman
// Use of this source code is governed by a Apache-style
// license found in the LICENSE file.  It also can be found at
// https://github.com/pborman/pdp8/blob/master/LICENSE

package os8fs

import (
	"fmt"
	"reflect"
	"unsafe"
)

// nativeOrder is true if we can read the bytes from the image file directly as
// a slice of uint16.  If so, we can make raw2words and words2raw faster.  If
// not, we have to copy them the hard way.
var nativeOrder = func() bool {
	raw := []byte{1, 2}
	var words []uint16
	rhdr := (*reflect.SliceHeader)(unsafe.Pointer(&raw))
	whdr := (*reflect.SliceHeader)(unsafe.Pointer(&words))
	whdr.Data = rhdr.Data
	whdr.Len = 1
	whdr.Cap = 1
	return words[0] == 0x201
}()

func raw2words(raw []byte) (words []uint16) {
	if nativeOrder {
		rhdr := (*reflect.SliceHeader)(unsafe.Pointer(&raw))
		whdr := (*reflect.SliceHeader)(unsafe.Pointer(&words))
		whdr.Data = rhdr.Data
		whdr.Len = rhdr.Len / 2
		whdr.Cap = rhdr.Cap / 2
	} else {
		words = make([]uint16, len(raw)/2)
		for i := range words {
			words[i] = uint16(raw[i*2]) | uint16(raw[i*2+1])<<8
		}
	}
	return words
}

func words2raw(words []uint16) (raw []byte) {
	if nativeOrder {
		rhdr := (*reflect.SliceHeader)(unsafe.Pointer(&raw))
		whdr := (*reflect.SliceHeader)(unsafe.Pointer(&words))
		rhdr.Data = whdr.Data
		rhdr.Len = whdr.Len * 2
		rhdr.Cap = whdr.Cap * 2
	} else {
		raw = make([]byte, len(words)*2)
		for i, w := range words {
			raw[i*2] = byte(w)
			raw[i*2+1] = byte(w >> 8)
		}
	}
	return raw
}

// A Date represents a date stamp on an OS/8 Fielsystem.
// Dates are limited to 1970-1977.
type Date uint16

var months = []string{
	"M0",
	"JAN", "FEB", "MAR", "APR", "MAY", "JUN", "JUL", "AUG", "SEP", "OCT", "NOV", "DEC",
	"M13", "M14", "M15",
}

// A Drive represents a disk drive.
type Drive struct {
	Tracks     int
	Sectors    int
	SectorSize int // in words
	Bytes      int // image size (per side)
	Sides      int
}

// Various know drive types for the PDP-8.
var (
	RK05 = Drive{Tracks: 204, Sectors: 16, SectorSize: 256, Bytes: 1662976, Sides: 2}
	RX01 = Drive{Tracks: 77, Sectors: 26, SectorSize: 64, Bytes: 256256, Sides: 1}
	RX02 = Drive{Tracks: 77, Sectors: 26, SectorSize: 128, Bytes: 512512, Sides: 1}
	DF32 = Drive{Tracks: 16, Sectors: 1, SectorSize: 2048, Bytes: 65536, Sides: 4}

	// Generic is a generic drive with a single side.  The size of the drive is
	// determined by the size of the disk image.
	Generic = Drive{}
)

func (d Date) String() string {
	if d == 0 {
		return ""
	}
	month := int(d>>8) & 0xf
	day := int(d>>3) & 0x1f
	year := int(d & 07)
	return fmt.Sprintf("%02d-%s-%d", day, months[month], year+0106)
}

type dirBlock struct {
	nfiles uint16 // 010000 - nfiles is number of files in block
	block0 uint16 // first block of data
	next   uint16 // next block in directory
	header [2]uint16
	data   [0400 - 5]uint16
}

// A fileEntry represents the internal structure of a single file entry.
type fileEntry struct {
	name [4]uint16 // 6 byte filename w/2 byte extension as 6 bit ascii
	date Date      // date stamp
	len  uint16    // 10000 - len is real len
}

func (f fileEntry) Name() string {
	var res [9]byte
	i := 0
	for _, w := range f.name[:3] {
		a := ASCII6(w)
		if a[0] == '@' {
			break
		}
		res[i] = a[0]
		i++
		if a[1] == '@' {
			break
		}
		res[i] = a[1]
		i++
	}
	if a := ASCII6(f.name[3]); a[0] != '@' {
		res[i] = '.'
		i++
		res[i] = a[0]
		i++
		if a[1] != '@' {
			res[i] = a[1]
			i++
		}
	}
	return string(res[:i])
}

func (f fileEntry) Len() int {
	return 010000 - int(f.len)
}

// ASCII6 returns w as 2 ascii bytes.
func ASCII6(w uint16) (a [2]byte) {
	a[0] = byte((w >> 6) & 0x3f)
	if a[0] < 32 {
		a[0] += 64
	}
	a[1] = byte(w & 0x3f)
	if a[1] < 32 {
		a[1] += 64
	}
	return a
}

// ASCII8 writes the first two words of src as 3 bytes into dst, masking each
// byte with m.
func ASCII8(dst []byte, src []uint16, m byte) {
	dst[0] = byte(src[0]) & m
	dst[1] = byte(src[1]) & m
	dst[2] = byte(((src[0]>>4)&0xf0)|((src[1]>>8)&0xf)) & m
}
