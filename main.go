package main

import (
	"github.com/MSOpenTech/azure-sdk-for-go/storage"
	"path"

	"github.com/paulmey/inspect-azure-vhd/ext4"

	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"net/http"
)

const (
	apiVersion = "2014-02-14"
)

var (
	help bool
)

func init() {
	flag.BoolVar(&help, "help", false, "Prints this help")
}

func main() {
	flag.Parse()
	if flag.NArg() != 1 || help {
		fmt.Printf("Usage: ./inspect-remote-vhd <vhd-read-uri>")
		flag.PrintDefaults()
		return
	}

	uri := flag.Arg(0)

	s := SasPageBlobAccessor(uri)

	fmt.Printf("Reading partition table...\n")
	// location of MBR partition table http://en.wikipedia.org/wiki/Master_boot_record#Sector_layout
	s.Seek(446, 0)
	var partitions [4]ext4.PartitionEntry
	err := binary.Read(s, binary.LittleEndian, &partitions)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Inspecting ext4 filesystem on first partition...\n")
	// assume that partition 0 is linux with ext4
	r, err := ext4.NewExtReader(s, partitions[0])
	if err != nil {
		panic(err)
	}

	globs := []string{
		"/etc/ssh*/*",
		"/etc/ssh*",
		"/etc/fstab",
		"/etc/mtab",
		"/etc/waagent.conf",
		"/var/log/messages",
		"/var/log/boot.log",
		"/var/log/dmesg",
		"/var/log/syslog",
		"/var/log/waagent/*",
		"/var/log/waagent*",
		"/var/log/walinuxagent/*",
		"/var/log/walinuxagent*",
		"/var/log/azure/*",
		"/var/log/*",
	}
	cache := map[string][]ext4.DirEntry{}

	fmt.Printf("Downloading interesting files...\n")
	for _, glob := range globs {
		i := len(glob)
		for ; i > 0 && glob[i-1] != '/'; i-- {
		}
		p := glob[:i]
		file := glob[i:]
		fmt.Printf("  looking for %s %s\n", p, file)

		entries, ok := cache[p]
		if !ok {
			entries, err = r.ListPath(p)
			if err == ext4.ErrNotFound {
				cache[p] = []ext4.DirEntry{}
			} else if err != nil {
				panic(err)
			}
			cache[p] = entries
		}
		for _, de := range entries {
			if ok, err := path.Match(file, de.Name.String()); err != nil {
				fmt.Printf("  error trying to match files:", err)
			} else if ok && (de.FileType == ext4.FileTypeFile || de.FileType == ext4.FileTypeSymlink) {
				fmt.Printf("    %s%s (%s)\n", p, de.Name, de.FileType)
			}
		}
	}
}

func SasPageBlobAccessor(url string) io.ReadSeeker {
	return &readSeekablePageBlob{
		url: url,
	}
}

type readSeekablePageBlob struct {
	url    string
	offset int64
}

func (b *readSeekablePageBlob) Read(buffer []byte) (n int, err error) {
	if len(buffer) == 0 {
		return
	}

	req, err := http.NewRequest("GET", b.url, nil)
	if err != nil {
		return
	}
	req.Header.Set("x-ms-version", apiVersion)
	req.Header.Set("x-ms-range", fmt.Sprintf("bytes=%d-%d", b.offset, b.offset+int64(len(buffer))))

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	if !(res.StatusCode == http.StatusOK ||
		res.StatusCode == http.StatusPartialContent) {
		return 0, fmt.Errorf("Non success status code: %s", res.Status)
	}

	defer res.Body.Close()

	// paulmey: for some reason, ioutil.ReadAll reads on infinitely on res.Body ?
	for n < len(buffer) && err == nil {
		nn, nerr := res.Body.Read(buffer[n:])
		err = nerr
		n += nn
	}
	//	fmt.Printf("### read %d bytes, err=%v\n", n, err)
	b.offset += int64(n)
	return
}

func (b *readSeekablePageBlob) Seek(offset int64, whence int) (int64, error) {
	if offset < 0 {
		return 0, fmt.Errorf("Cannot seek with negative offset: %d", offset)
	}
	if whence < 0 || whence > 2 {
		return 0, fmt.Errorf("Illegal value for parameter whence: %s", whence)
	}

	switch whence {
	case 0:
		if offset != b.offset {
			props, err := b.getProperties()
			if err != nil {
				return 0, err
			}

			if offset > props.ContentLength {
				return 0, fmt.Errorf("Cannot seek beyond end of blob (%d > %d)", offset, props.ContentLength)
			}
			b.offset = offset
		}
	case 1:
		if offset != 0 {
			props, err := b.getProperties()
			if err != nil {
				return 0, err
			}

			if b.offset+offset > props.ContentLength {
				return 0, fmt.Errorf("Cannot seek beyond end of blob (%d > %d)", b.offset+offset, props.ContentLength)
			}
			b.offset += offset
		}
	case 2:
		if offset != 0 {
			return 0, fmt.Errorf("Cannot seek beyond end of blob")
		}

		props, err := b.getProperties()
		if err != nil {
			return 0, err
		}
		b.offset = props.ContentLength
	default:
		return 0, errNotImplemented
	}

	return b.offset, nil
}

func (b readSeekablePageBlob) getProperties() (storage.BlobProperties, error) {
	var rv storage.BlobProperties

	req, err := http.NewRequest("HEAD", b.url, nil)
	if err != nil {
		return rv, err
	}
	req.Header.Set("x-ms-version", apiVersion)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return rv, err
	}
	if !(res.StatusCode == http.StatusOK) {
		return rv, fmt.Errorf("Non success status code: %s", res.Status)
	}
	defer res.Body.Close()

	rv.BlobType = storage.BlobType(res.Header.Get("x-ms-blob-type"))
	fmt.Sscanf(res.Header.Get("Content-Length"), "%d", &rv.ContentLength)
	return rv, nil
}

var errNotImplemented = fmt.Errorf("Not implemented")
