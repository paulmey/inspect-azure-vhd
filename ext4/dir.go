package ext4

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"regexp"
	"strings"
)

func (er Reader) Root() (Directory, error) {
	inode, err := er.GetInode(2)
	if err != nil {
		return Directory{}, err
	}
	return Directory{
		r:     er,
		inode: inode,
		path:  "/",
	}, nil
}

type Directory struct {
	r     Reader
	inode Inode
	path  string
}

func (d Directory) Entries() ([]DirEntry, error) {
	var entries []DirEntry
	b, err := d.r.GetInodeContent(d.inode)
	if err != nil {
		return entries, err
	}

	entries = make([]DirEntry, 0, d.r.super.blockSize()/12) // min dir_entry2 rec_len seems to be 12
	r := bytes.NewReader(b)
	for {
		de, err := readDirEntry(r)
		if err == io.EOF {
			break
		}
		if err != nil {
			return []DirEntry{}, err
		}
		entries = append(entries, de)
	}
	return entries, nil
}

func (d Directory) findEntry(name string) (entry DirEntry, err error) {
	entries, err := d.Entries()
	if err != nil {
		return DirEntry{}, err
	}
	for _, e := range entries {
		if string(e.Name) == name {
			return e, nil
		}
	}
	return DirEntry{}, ErrNotFound
}

var slashes = regexp.MustCompile("/+")

func (d Directory) ChangeDir(path string) (Directory, error) {
	path = slashes.ReplaceAllLiteralString(path, "/")
	s := strings.Split(path, "/")
	for len(s) > 0 && s[0] == "" {
		s = s[1:]
	}
	if len(s) == 0 {
		return Directory{}, fmt.Errorf("invalid path")
	}
	e, err := d.findEntry(s[0])
	if err != nil {
		return Directory{}, err
	}
	if e.FileType == FileTypeDir {
		inode, err := d.r.GetInode(e.Inode)
		if err != nil {
			return Directory{}, err
		}
		dir := Directory{
			r:     d.r,
			inode: inode,
			path:  d.path + s[0] + "/",
		}
		if len(s) == 1 {
			return dir, nil
		}
		return dir.ChangeDir(strings.Join(s[1:], "/"))
	}
	if e.FileType == FileTypeSymlink {
		inode, err := d.r.GetInode(e.Inode)
		if err != nil {
			return Directory{}, err
		}
		link, err := ioutil.ReadAll(inode.GetDataReader())
		if err != nil {
			return Directory{}, err
		}
		path = string(link) + "/" + strings.Join(s[1:], "/")
		return d.ChangeDir(path)
	}
	return Directory{}, fmt.Errorf("Not a directory or symlink: ", d.path+s[0])
}

func readDirEntry(r io.Reader) (entry DirEntry, err error) {
	err = binary.Read(r, binary.LittleEndian, &entry.DirEntryHeader)
	if err != nil {
		return
	}
	entry.Name = make(charArray, entry.NameLen, entry.NameLen)
	err = binary.Read(r, binary.LittleEndian, &entry.Name)
	if err != nil {
		return
	}
	buf := make([]byte, entry.RecLen-8-uint16(entry.NameLen))
	_, err = r.Read(buf)
	return
}

type DirEntry struct {
	DirEntryHeader
	Name charArray // File name.
}

type charArray []byte

func (c charArray) String() string {
	return string([]byte(c))
}

type DirEntryHeader struct {
	Inode   uint32 // Number of the inode that this directory entry points to.
	RecLen  uint16 // Length of this directory entry.
	NameLen byte   // Length of the file name.
	// File type code, one of:
	FileType FileType
}

type FileType byte

const (
	FileTypeUnknown  FileType = 0x0 // Unknown.
	FileTypeFile     FileType = 0x1 // Regular file.
	FileTypeDir      FileType = 0x2 // Directory.
	FileTypeChardev  FileType = 0x3 // Character device file.
	FileTypeBlockdev FileType = 0x4 // Block device file.
	FileTypeFIFO     FileType = 0x5 // FIFO.
	FileTypeSocket   FileType = 0x6 // Socket.
	FileTypeSymlink  FileType = 0x7 // Symbolic link.
)

func (t FileType) String() string {
	switch t {
	case FileTypeUnknown:
		return "Unknown"
	case FileTypeFile:
		return "File"
	case FileTypeDir:
		return "Dir"
	case FileTypeChardev:
		return "Chardev"
	case FileTypeBlockdev:
		return "Blockdev"
	case FileTypeFIFO:
		return "FIFO"
	case FileTypeSocket:
		return "Socket"
	case FileTypeSymlink:
		return "Symlink"
	default:
		return fmt.Sprintf("FileType(0x%x)", t)
	}
}
