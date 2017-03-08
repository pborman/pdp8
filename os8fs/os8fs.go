// Copyright 2017 Paul Borman
// Use of this source code is governed by a Apache-style
// license found in the LICENSE file.  It also can be found at
// https://github.com/pborman/pdp8/blob/master/LICENSE

// Package os8fs provides support for reading OS/8 Filesystems and disk images
// used by PDP-8 computers.
//
// Disk images may contain multiple sides, each with their own filesystem
// (typically there are only 1 or 2 sides).  Filenames may be prefixed by X: to
// indicate side X of the disk should be used.  Sides are numbered starting with
// A (e.g., A: and B:).  If a side is not indicated then the first side is
// assumed.
//
// The OS/8 Filesystem is a flat filesystem, each file being contiguous on disk.
// A filesystem may have up to 4,096 blocks.  Each block is composed of 256 12
// bit words.  Image files use 2 bytes per word with the first byte being the
// lower 8 bits of the 12 bit word and the second by being the upper 4 bits.
// The bits uuuullllllll are stored as:
//
//  +--------+--------+
//  |llllllll|0000uuuu|
//  +--------+--------+
//
// File are listed in chained directory blocks starting with block 1 (block 0 is
// presumed to be a boot block).  Each directory block starts with a header of 5
// 12 bit words:
//
//   +------------+
//   |010000-nfile|   Number of files in the directory block
//   +------------+
//   | BLOCK0     |   Index of first data block of first entry
//   +------------+
//   | NEXT BLOCK |   Index of next directory block (0 means end)
//   +------------+
//   |     ?      |   Unknown
//   +------------+
//   |     ?      |   Unknown
//   +------------+
//
// Numbers are often stored as 010000 - N on a PDP-8 rather than as N.  This
// enables using the ISZ instruction to execute a loop N times.
//
// Following the 5 word header, up to 40 directory entries are listed.  The data
// associated with these entries are contiguous on disk, starting with BLOCK0.
// Directory entires are either 2 or 6 words long and packed (e.g.,
// 11111122333333).  2 word entries always start with a 0 word and represent
// free space (a deleted file).  The second word is the number of blocks free:
//
//   +------------+
//   |000000000000|
//   +------------+
//   |010000 - len|
//   +------------+
//
// 6 word entries represent actual files:
//   +------+------+
//   | NAME | NAME |
//   +------+------+
//   | NAME | NAME |
//   +------+------+
//   | NAME | NAME |
//   +------+------+
//   | EXT  | EXT  |
//   +------+------+
//   | DATE        |
//   +-------------+
//   | 010000 - len|
//   +-------------+
//
// The NAME and EXT are stored as 6 bit ascii.  Each 6 bits represents one byte.
// Value 0-31 map to 64-95 and values 32-63 map as themselves (e.g., 01 is 'A'
// and 41 is '!').  NAME and EXT are padded with @ bytes (0).  For example, the
// name NAME.GO would be represented in octal as 1601 1505 0000 0717.
//
// The location of a file is computed by starting with the data block index from
// the directory block header and then adding all the sizes of all files, and
// deleted entries, listed in the directory block before the file's entry.
//
// Dates are stored as MMMMDDDDDYYY (4 bit month, 5 bit day, 3 bit year).  The
// year is offset from 1970 so only dates in the range of 1970-1977 can be
// represented.  A 0 date indicates the file has no date associated with it.
//
// When a file is deleted, it's entry is collapsed to two word, a 0 and its
// length.  The remaining entries in the directory block are all moved up by 4
// words.
//
// Text files normally store 3 ASCII bytes as two words.  The bytes aaaaaaaa,
// bbbbbbbb, ccccCCCC is stored as:
//
//  +------------+
//  +ccccaaaaaaaa|
//  +------------+
//  +CCCCbbbbbbbb|
//  +------------+
package os8fs

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"unsafe"
)

// DefaultImage is the name of the PDP-8 OS/8 image to use if not image
// is specified.  By default this is the value of the PDP8_IMAGE environment
// variable.
var DefaultImage = os.Getenv("PDP8_IMAGE")

// A FileSystem represents a single OS/8 filesystem as found on a PDP-8 disk.
type FileSystem struct {
	fd      *os.File // file descriptor of open image
	block0  int      // offset to block0 of the image
	nblocks int      // number of 256 word blocks on filesystem
}

// A FileInfo contains metadata about a single file in an OS/8 filesystem.
type FileInfo struct {
	Name   string // Name of the file
	Date   Date   // Date of the file (may be 0)
	Size   int    // Number of 256 word blocks in the file
	Offset int    // Block number to the start of the files data
}

// A File represents a file in an OS/8 FileSystem.
type File struct {
	fs     *FileSystem
	name   string
	date   Date
	size   int // size of file in 256 word blocks
	dir    int // block offset of directory block
	loc    int // location of the directory entry in directory block
	offset int // first block of the files data
	words  []uint16
}

// Bytes returns the contents of f as raw bytes (2 bytes per word, second byte
// has only 4 bits of meaningful data).
func (f *File) Bytes() []byte {
	return words2raw(f.words)
}

// Words returns the contents of f as 12 bit words.
func (f *File) Words() []uint16 {
	return f.words
}

// Name returns the name of the file.
func (f *File) Name() string {
	return f.name
}

// ASCII returns the contents of f as 7 bit ASCII encoded as 3 bytes per 2
// words.  If strip is true, the 8th bit of each byte is stripped.
func (f *File) ASCII(strip bool) []byte {
	m := byte(0xff)
	if strip {
		m = 0x7f
	}
	words := f.Words()
	ascii := make([]byte, 3*len(words)/2)
	for i := 0; i < len(words)/2; i++ {
		ASCII8(ascii[i*3:], words[i*2:], m)
	}
	for i := len(ascii); i > 0; i-- {
		if ascii[i-1] != 0 {
			return ascii[:i]
		}
	}
	return nil
}

// ASCII6 returns the contents of f as 6 bit ASCII encoded as 2 bytes per word.
func (f *File) ASCII6() []byte {
	words := f.Words()
	ascii := make([]byte, len(words)*2)
	for i, w := range words {
		a := ASCII6(w)
		ascii[i*2] = a[0]
		ascii[i*2+1] = a[1]
	}
	for i := len(ascii); i > 0; i-- {
		if ascii[i-1] != '@' {
			return ascii[:i]
		}
	}
	return nil
}

// A Disk represents a single disk with one or more filesystems.
type Disk struct {
	path  string   // location of source file
	fd    *os.File // actual image
	drive Drive    // Drive information
	sides []*FileSystem
}

// OpenImage opens path as a PDP-8 disk image.  The disk type is automatically
// determined by path's extension (.rk05, .rx01, or .rx02).  If rw is true then
// the image is opened read/write.
//
// If the disk image contains more than one side, the side number is specified
// by using A: or B: as a prefix to the name.  A missing side prefix is taken as
// the first side.
//
// If no extension is provided, or the extension is unknown, path is assumed to
// contain a single OS/8 filesystem.
//
// If the path is empty, DefaultImage is used.
func OpenImage(path string, rw bool) (*Disk, error) {
	if path == "" {
		path = DefaultImage
		if path == "" {
			return nil, ErrNotPath
		}
	}
	switch strings.ToUpper(filepath.Ext(path)) {
	case ".RK05":
		return RK05.OpenImage(path, rw)
	case ".RX01":
		return RX01.OpenImage(path, rw)
	case ".RX02":
		return RX02.OpenImage(path, rw)
	default:
		return Generic.OpenImage(path, rw)
	}
}

// ErrNotPath is returned if GetFile is passed a path that does not contain
// both a drive image and file name.
var ErrNotPath = errors.New("no path to drive")

// GetFile returns the file named by the base name of path on the disk image
// specified by the directory part of path.  E.g. os8.rk05/A:INIT.TX refers to
// the file named INIT.TX on the first side of the disk image os8.rk05.  The
// type of disk is intuited from the image name.
func GetFile(path string) (*File, error) {
	if strings.LastIndex(path, "/") < 0 {
		if DefaultImage == "" {
			return nil, ErrNotPath
		}
		path = filepath.Join(DefaultImage, path)
	}
	switch strings.ToUpper(filepath.Ext(filepath.Dir(path))) {
	case ".RK05":
		return RK05.GetFile(path)
	case ".RX01":
		return RX01.GetFile(path)
	case ".RX02":
		return RX02.GetFile(path)
	default:
		return Generic.GetFile(path)
	}
}

// GetFile is like the function GetFile but the disk type is specified by d.
func (d Drive) GetFile(path string) (*File, error) {
	image := DefaultImage
	if x := strings.LastIndex(path, "/"); x >= 0 {
		image = path[:x]
		path = path[x+1:]
	}
	if image == "" {
		return nil, ErrNotPath
	}
	disk, err := d.OpenImage(image, false)
	if err != nil {
		return nil, err
	}
	return disk.File(path)
}

var (
	driveMu sync.Mutex
	drives  = map[string]*Disk{}
)

// OpenImage opens path as a disk drive of type d returning either the opened
// disk or an error.  If rw is true, open the image read/write.
func (d Drive) OpenImage(path string, rw bool) (_ *Disk, err error) {
	if path == "" {
		path = DefaultImage
		if path == "" {
			return nil, ErrNotPath
		}
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	driveMu.Lock()
	defer driveMu.Unlock()
	if disk := drives[path]; disk != nil {
		return disk, nil
	}
	var fd *os.File
	if rw {
		fd, err = os.OpenFile(path, os.O_RDWR, 0)
	} else {
		fd, err = os.Open(path)
	}
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			fd.Close()
		}
	}()
	fi, err := fd.Stat()
	if err != nil {
		return nil, err
	}
	if d.Sides == 0 {
		d.Sides = 1
	}
	if d.Bytes == 0 {
		d.Bytes = int(fi.Size()) / d.Sides
	}
	if d.Bytes > int(fi.Size()) {
		return nil, fmt.Errorf("truncated image (%d < %d): %s", fi.Size(), d.Bytes, path)
	}

	// We have at least one side, see how many sides are in the file
	for d.Bytes*d.Sides < int(fi.Size()) {
		d.Sides--
	}
	data := make([]byte, d.Bytes*d.Sides)
	if _, err := io.ReadFull(fd, data); err != nil {
		return nil, err
	}

	disk := Disk{
		path:  path,
		drive: d,
		fd:    fd,
		sides: make([]*FileSystem, d.Sides),
	}
	for s := range disk.sides {
		disk.sides[s] = &FileSystem{
			fd:      fd,
			block0:  s * d.Bytes >> 9,
			nblocks: d.Bytes >> 9,
		}
	}
	drives[path] = &disk
	return &disk, nil
}

func (d *Disk) Close() error {
	driveMu.Lock()
	delete(drives, d.path)
	driveMu.Unlock()
	// TODO: write out any changes.
	return nil
}

func (d *Disk) getFS(name string) (*FileSystem, string) {
	if len(name) > 2 && name[1] == ':' {
		n := (int(name[0]) | 040) - 'a'
		if n < 0 || n > len(d.sides) {
			return nil, name
		}
		return d.sides[n], name[2:]
	}
	return d.sides[0], name
}

// File returns information about the specified file on d, or an error.  A
// missing drive prefix is assumed to be A:.
func (d *Disk) File(name string) (*File, error) {
	fs, name := d.getFS(name)
	if fs == nil {
		return nil, fmt.Errorf("side not found: %s", name)
	}
	return fs.File(name)
}

func (f *FileSystem) getBlocks(start, cnt int) ([]uint16, error) {
	if cnt <= 0 {
		return nil, fmt.Errorf("getBlocks: non-positive count(%d)", cnt)
	}
	if start < 0 {
		return nil, fmt.Errorf("getBlocks: negative start(%d)", start)
	}
	if start+cnt > f.nblocks {
		return nil, fmt.Errorf("getBlocks: block out of range(%d > %d", start+cnt, f.nblocks)
	}
	data := make([]byte, cnt*512)
	n, err := f.fd.ReadAt(data, int64(f.block0+start)*512)
	if err != nil {
		return nil, fmt.Errorf("getBlocks(%d,%d): %v", f.block0+start, cnt, err)
	}
	if n < cnt*512 {
		return nil, fmt.Errorf("getBlocks: truncated read")
	}
	return raw2words(data), nil
}

func (f *FileSystem) writeBlocks(start int, words []uint16) error {
	if len(words) == 0 {
		return nil
	}
	cnt := len(words) / 256
	if cnt*256 != len(words) {
		return fmt.Errorf("writeBlocks: invalid block size(%d)", len(words))
	}
	if start < 0 {
		return fmt.Errorf("writeBlocks: negative start(%d)", start)
	}
	if start+cnt > f.nblocks {
		return fmt.Errorf("writeBlocks: block out of range(%d > %d", start+cnt, f.nblocks)
	}
	data := words2raw(words)
	_, err := f.fd.WriteAt(data, int64(f.block0+start)*512)
	return err
}

// File returns information about the specified file on f, or an error.  File
// names starting with .block represent raw blocks of data in the file system.
// There are two forms: .blockS and .blockS-E where S is the initial block
// number and E is the ending block number.
func (f *FileSystem) File(name string) (*File, error) {
	if name == "" {
		return nil, errors.New("missing filename")
	}
	name = strings.ToUpper(name)
	const (
		dotBlock = ".BLOCK"
	)
	if strings.HasPrefix(name, dotBlock) {
		starts := name[len(dotBlock):]
		ends := starts
		if x := strings.Index(starts, "-"); x >= 0 {
			ends = starts[x+1:]
			starts = starts[:x]
		}
		start, err := strconv.ParseUint(starts, 0, 16)
		if err != nil {
			return nil, fmt.Errorf("invalid filename: %s", name)
		}
		end, err := strconv.ParseUint(ends, 0, 16)
		if err != nil {
			return nil, fmt.Errorf("invalid filename: %s", name)
		}
		if end < start || int(start) >= f.nblocks {
			return nil, fmt.Errorf("invalid block range: %s", name)
		}
		if int(end) >= f.nblocks {
			end = uint64(f.nblocks - 1)
		}
		size := int(end - start + 1)
		words, err := f.getBlocks(int(start), size)
		if err != nil {
			return nil, fmt.Errorf("%v: %s", err, name)
		}
		return &File{
			fs:     f,
			name:   name,
			size:   size,
			offset: int(start),
			words:  words,
		}, nil
	}
	var file *File
	if err := f.scan(func(sd *scanData) error {
		if sd.file == nil || sd.file.Name() != name {
			return nil
		}
		words, err := f.getBlocks(sd.block0, sd.size)
		if err != nil {
			return fmt.Errorf("%v: %s", err, name)
		}
		file = &File{
			fs:     f,
			name:   name,
			date:   sd.file.date,
			size:   sd.size,
			loc:    sd.loc,
			dir:    sd.index,
			offset: sd.block0,
			words:  words,
		}
		return stopReading
	}); err != nil {
		return nil, err
	}
	if file != nil {
		return file, nil
	}
	return nil, fmt.Errorf("file not found: %s", name)
}

// List returns a list FileInfos for every file on d.  If d contains multiple
// sides then file names will contain a drive prefix.
func (d *Disk) List() ([]FileInfo, error) {
	if len(d.sides) == 0 {
		return nil, nil
	}
	var cfis []FileInfo
	for s, fs := range d.sides {
		fis, err := fs.List()
		if err != nil {
			return cfis, err
		}
		if len(d.sides) > 1 {
			for i, fi := range fis {
				fis[i].Name = fmt.Sprintf("%c:%s", s+'A', fi.Name)
			}
		}
		cfis = append(cfis, fis...)
	}
	return cfis, nil
}

// // List returns a list FileInfos for every file on f.
func (f *FileSystem) List() ([]FileInfo, error) {
	var fis []FileInfo
	err := f.scan(func(sd *scanData) error {
		if sd.file != nil {
			fis = append(fis, FileInfo{
				Name:   sd.file.Name(),
				Date:   sd.file.date,
				Size:   sd.size,
				Offset: sd.block0,
			})
		}
		return nil
	})
	return fis, err
}

var (
	stopReading = errors.New("stop reading")
	skipBlock   = errors.New("skip block")
)

// scanData is passed to the scan callback function with information
// about the current scan.  The file field is nil if the
// directory entry represents free space.
type scanData struct {
	index  int        // index of directory block
	loc    int        // location of entry in directory block
	block0 int        // first block of data for the file
	size   int        // size of file
	words  []uint16   // directory block data
	block  *dirBlock  // words as a dirBlock
	file   *fileEntry // actual entry
}

// Scan scans the directory entries in f calling cb for each directory entry
// found.  cb is passed scanData.  Scan continues until all entries are read, cb
// returns an error, or an error is encountered.  The error stopReading stops
// the scan, but does not return an error.
func (f *FileSystem) scan(cb func(*scanData) error) (err error) {
	var block *dirBlock
	for index := 1; index != 0; index = int(block.next) {
		words, err := f.getBlocks(index, 1)
		if err != nil {
			return err
		}
		hdr := (*reflect.SliceHeader)(unsafe.Pointer(&words)) // case 1
		block = (*dirBlock)(unsafe.Pointer(hdr.Data))

		nfiles := int(010000 - block.nfiles)
		if nfiles > 40 {
			return fmt.Errorf("directory block %d: too many entries: %d", index, nfiles)
		}
		block0 := int(block.block0)

		// loc is the word index to the next directory entry.
		loc := +5
	Reading:
		for i := 0; i < nfiles; i++ {
			edata := words[loc:]
			if edata[0] == 0 {
				n := int(010000 - edata[1])
				err := cb(&scanData{
					index:  index,
					loc:    loc,
					block:  block,
					block0: block0,
					size:   n,
					words:  words,
				})
				switch err {
				case nil:
				case stopReading:
					return nil
				case skipBlock:
					break Reading
				default:
					return err
				}
				block0 += n
				loc += 2
				continue
			}
			hdr := (*reflect.SliceHeader)(unsafe.Pointer(&edata))
			e := (*fileEntry)(unsafe.Pointer(hdr.Data))
			if block0+e.Len() > f.nblocks {
				return fmt.Errorf("corrupt directory, block out of range (%d)", block0+e.Len())
			}
			err := cb(&scanData{
				index:  index,
				loc:    loc,
				block:  block,
				block0: block0,
				size:   e.Len(),
				words:  words,
				file:   e,
			})
			switch err {
			case nil:
			case stopReading:
				return nil
			case skipBlock:
				break Reading
			default:
				return err
			}
			block0 += e.Len()
			loc += 6
		}
	}
	return nil
}

// Remove removes the named file from d.  The filename may be preceded
// by A: or B: to indicate which side of the disk should be used.
// THIS IS EXPERIMENTAL!
func (d *Disk) Remove(name string) error {
	fs, name := d.getFS(name)
	if fs == nil {
		return fmt.Errorf("side not found: %s", name)
	}
	return fs.Remove(name)
}

// Remove removes the named file from f.
// THIS IS EXPERIMENTAL!
func (f *FileSystem) Remove(name string) error {
	name = strings.ToUpper(name)
	found := false
	var werr error
	err := f.scan(func(sd *scanData) error {
		if sd.file == nil || sd.file.Name() != name {
			return nil
		}
		found = true
		sd.words[sd.loc] = 0
		sd.words[sd.loc+1] = uint16(010000 - sd.size)
		copy(sd.words[sd.loc+2:], sd.words[sd.loc+6:])
		werr = f.writeBlocks(sd.index, sd.words)
		return stopReading
	})
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("file not found: %s", name)
	}
	return werr
}
