package ext4

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
)

func (er ExtReader) ListPath(path string) ([]Ext4DirEntry2, error) {
	if path == "" || path[0] != '/' {
		return []Ext4DirEntry2{}, fmt.Errorf("path must start with '/': %q", path)
	}

	rootNode, err := er.GetInode(2)
	if err != nil {
		return []Ext4DirEntry2{}, err
	}

	return er.traversePath(rootNode, path)
}

func (er ExtReader) traversePath(current Ext4Inode, path string) (entries []Ext4DirEntry2, err error) {
	b, err := er.GetInodeContent(current)
	if err != nil {
		return
	}

	entries = make([]Ext4DirEntry2, 0, er.super.blockSize()/12) // min dir_entry2 rec_len seems to be 12
	r := bytes.NewReader(b)
	for {
		de, err := ReadDirectoryEntry2(r)
		if err == io.EOF {
			break
		}
		if err != nil {
			return []Ext4DirEntry2{}, err
		}
		entries = append(entries, de)
	}

	for path != "" && path[0] == '/' {
		path = path[1:]
	}
	if path == "" {
		return
	}

	i := 0
	for ; i < len(path) && path[i] != '/'; i++ {
	}
	dirname := path[:i]

	for _, e := range entries {
		if e.Name.String() == dirname {
			inode, err := er.GetInode(e.Inode)
			if err != nil {
				return []Ext4DirEntry2{}, err
			}
			if e.FileType == FileTypeDir {
				return er.traversePath(inode, path[i:])
			} else if e.FileType == FileTypeSymlink {
				if inode.Size() < 60 {
					l := string(inode.Data[:inode.Size()])
					//fmt.Printf("=== following symlink %s\n", l)
					return er.traversePath(current, l)
				} else {
					b, err := ioutil.ReadAll(inode.GetDataReader())
					if err != nil {
						return []Ext4DirEntry2{}, err
					}
					l := string(b)
					//fmt.Printf("=== following symlink %s\n", l)
					return er.traversePath(current, l)
				}
			}
		}
	}

	return []Ext4DirEntry2{}, ErrNotFound
}

func ReadDirectoryEntry2(r io.Reader) (entry Ext4DirEntry2, err error) {
	err = binary.Read(r, binary.LittleEndian, &entry.Ext4DirEntry2Header)
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

type Ext4DirEntry2 struct {
	Ext4DirEntry2Header
	Name charArray // File name.
}

type charArray []byte

func (c charArray) String() string {
	return string([]byte(c))
}

type Ext4DirEntry2Header struct {
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
