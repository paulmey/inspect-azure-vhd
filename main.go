package main

import (
	"github.com/MSOpenTech/azure-sdk-for-go/storage"

	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	//"time"
)

const (
	//	uri        = "https://pmwus123.blob.core.windows.net/images/PackerMade_OpenLogicImage_2015-April-25_19-40-os-2015-04-26.vhd?se=2015-06-01T00%3A00%3A00Z&sp=r&sv=2014-02-14&sr=b&sig=96R8sepchondTMF0ihlKkMRda0u1%2FvuSuvfZT6LKDuc%3D"
	apiVersion = "2014-02-14"
	pageSize   = 512
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
	var partitions [4]partitionEntry
	err := binary.Read(s, binary.LittleEndian, &partitions)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Inspecting ext4 filesystem on first partition...\n")
	// assume that partition 0 is linux with ext4
	r, err := NewExtReader(s, partitions[0])
	if err != nil {
		panic(err)
	}

	fmt.Printf("Iterating /var/log, downloading interesting files...\n")
	entries, err := r.ListPath("/var/log")
	if err != nil {
		panic(err)
	}
	for _, de := range entries {
		fmt.Printf("    %-10s %s", de.FileType, de.Name)
		if de.Name.String() == "messages" ||
			de.Name.String() == "boot.log" ||
			de.Name.String() == "dmesg" ||
			de.Name.String() == "syslog" {

			inode, err := r.GetInode(de.Inode)
			if err != nil {
				panic(err)
			}

			f, err := os.OpenFile(de.Name.String(), os.O_RDWR|os.O_CREATE, 0666)
			if err != nil {
				panic(err)
			}
			defer f.Close()

			ir, err := r.GetInodeReader(inode)
			if err != nil {
				panic(err)
			}

			n, err := io.Copy(f, ir)
			f.Close()

			fmt.Printf(" ==> downloaded %d bytes", n)
		}
		fmt.Println()
	}
}

type partitionEntry struct {
	Active   byte
	CHSFirst [3]byte
	Type     byte
	CHSLast  [3]byte
	LBAfirst uint32
	Sectors  uint32
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

	//fmt.Printf("[[ blob seek  %d -> ", b.offset)

	switch whence {
	case 0:
		if offset != b.offset {
			props, err := b.getProperties()
			if err != nil {
				return 0, err
			}

			if offset > props.ContentLength {
				return 0, fmt.Errorf("Cannot seek beyond end of blob")
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
				return 0, fmt.Errorf("Cannot seek beyond end of blob")
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

	//fmt.Printf("%d ]]", b.offset)
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
