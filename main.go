package main

import (
	"github.com/Azure/azure-sdk-for-go/storage"
	"io"
	"io/ioutil"
	"os"
	"path"

	"flag"
	"fmt"
	"github.com/paulmey/inspect-azure-vhd/ext4"
	"net/http"
)

const (
	apiVersion = "2014-02-14"
)

var (
	help      bool
	ouputPath string
)

func init() {
	flag.BoolVar(&help, "help", false, "Prints this help.")
	flag.StringVar(&ouputPath, "outputPath", "out", "Specifies the path where logs and files are placed.")
}

func main() {
	flag.Parse()
	if flag.NArg() != 1 || help {
		fmt.Printf("Usage: ./inspect-remote-vhd <vhd-read-uri>\n")
		flag.PrintDefaults()
		return
	}

	uri := flag.Arg(0)

	s := SasPageBlobAccessor(uri)

	fmt.Printf("Reading partition table...\n")
	// location of MBR partition table http://en.wikipedia.org/wiki/Master_boot_record#Sector_layout
	partitions, err := readPartitionTable(s)
	if err != nil {
		panic(err)
	}

	for partitionNum, p := range partitions {
		fmt.Printf("Inspecting filesystem on partition %d...\n", partitionNum)
		// assume that partition 0 is linux with ext4
		if p.Type != 0x83 {
			fmt.Printf("Not a linux partition (%d), skipping!\n", p.Type)
			continue
		}

		r, err := ext4.NewReader(s, p.LBAfirst, p.Sectors)
		if err == ext4.ErrNotExt4 {
			fmt.Printf("Filesystem is not ext4 compatible, skipping!\n")
			continue
		}
		if err != nil {
			panic(err)
		}

		globs := []string{
			"/etc/ssh*/*",
			"/etc/ssh*",
			"/etc/fstab",
			"/etc/passwd",
			"/etc/mtab",
			"/etc/waagent.conf",
			"/var/log/messages*",
			"/var/log/boot*",
			"/var/log/dmesg*",
			"/var/log/syslog*",
			"/var/log/waagent/*",
			"/var/log/waagent/*/*",
			"/var/log/waagent/*/*/*",
			"/var/log/waagent*",
			"/var/log/walinuxagent/*",
			"/var/log/walinuxagent/*/*",
			"/var/log/walinuxagent/*/*/*",
			"/var/log/walinuxagent*",
			"/var/log/azure/*",
			"/var/log/azure/*/*",
			"/var/log/azure/*/*/*",
			"/var/log/azure/*/*/*/*",
			"/var/log/*",
			"/boot/grub/*cfg",
			"/grub/*cfg",
		}

		fs, err := r.Root()
		if err != nil {
			panic(err)
		}

		fmt.Printf("Downloading interesting files...\n")
		for _, glob := range globs {
			files, err := fs.Match(glob)
			if err != nil {
				panic(err)
			}
			for _, f := range files {
				orig := f
				for f.FileType == ext4.FileTypeSymlink {
					f, err = f.ResolveSymlink()
					if err != nil {
						fmt.Printf("WARN: failed to resolve symlink %s: %v\n", orig.Fullname(), err)
						continue
					}
				}
				if f.FileType != ext4.FileTypeFile {
					continue
				}
				inode, err := r.GetInode(f.Inode)
				if err != nil {
					fmt.Printf("WARN: could not read inode %d (%s -> %s): %v\n", f.Inode, orig.Fullname(), f.Fullname(), err)
					continue
				}

				fmt.Printf("   %s (%s) \n", orig.Fullname(), orig.FileType)
				fmt.Printf("     \\-> downloading %d bytes\n", inode.Size())

				data, err := r.GetInodeContent(inode)
				if err != nil {
					fmt.Printf("WARN: could not read data for %s: %s", orig.Fullname(), err)
					continue
				}

				outFile := ouputPath + fmt.Sprintf("/%d/", partitionNum) + fixFilename(orig.Fullname())
				if err := os.MkdirAll(path.Dir(outFile), 0777); err != nil {
					fmt.Printf("ERR: could not create path %s: %s", path.Dir(outFile), err)
					return
				}
				err = ioutil.WriteFile(outFile, data, 0666)
				if err != nil {
					fmt.Printf("ERR: could not write file %s: %s", path.Dir(outFile), err)
					return
				}
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
