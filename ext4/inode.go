package ext4

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

func (er *Reader) GetInode(n uint32) (inode Inode, err error) {
	if n < 1 {
		err = fmt.Errorf("inode number (n) should be 1-based and positive")
	}
	n--
	bg := n / er.super.InodesPerGroup
	index := n % er.super.InodesPerGroup

	gd, err := er.GetGroupDescriptor(bg)
	if err != nil {
		return
	}
	if _, err = er.s.Seek(er.blockOffset(gd.InodeTableBlock())+int64(er.super.InodeSize)*int64(index), 0); err != nil {
		return
	}
	err = binary.Read(er.s, binary.LittleEndian, &inode)
	return
}

func (er *Reader) GetInodeReader(inode Inode) (io.Reader, error) {
	indr := inodeDataReader{
		er:     er,
		length: int64(inode.Size()),
	}
	extents, err := er.GetExtents(inode)
	if err != nil {
		return nil, err
	}
	indr.extents = extents
	return &indr, nil
}

func (er *Reader) GetInodeContent(inode Inode) ([]byte, error) {
	r, err := er.GetInodeReader(inode)
	if err != nil {
		return nil, err
	}
	b := make([]byte, inode.Size())
	_, err = r.Read(b)
	return b, err
}

type inodeDataReader struct {
	er      *Reader
	extents []Extent
	offset  int64
	length  int64
}

func (iodr *inodeDataReader) Read(b []byte) (n int, err error) {
	l := len(b)
	blockSize := iodr.er.super.blockSize()

	for _, extent := range iodr.extents {
		if n == l { // satisfied request
			return
		}
		if iodr.offset >= iodr.length { // file exhausted
			err = io.EOF
			return
		}

		extentStartOffset := int64(extent.Block) * blockSize
		extentEndOffset := int64(extent.Block+uint32(extent.Len)) * blockSize
		if iodr.offset < extentStartOffset ||
			iodr.offset >= extentEndOffset {
			continue
		}

		chunkSize := l - n
		leftInExtent := extentEndOffset - iodr.offset
		leftInNode := iodr.length - iodr.offset
		if int64(chunkSize) > leftInExtent {
			chunkSize = int(leftInExtent)
		}
		if int64(chunkSize) > leftInNode {
			chunkSize = int(leftInNode)
		}

		iodr.er.s.Seek(iodr.er.blockOffset(extent.Start())+
			iodr.offset-extentStartOffset, 0)

		nn, err := iodr.er.s.Read(b[n : n+chunkSize])
		iodr.offset += int64(nn)
		n += nn
		if err != nil {
			return n, err
		}
	}
	return
}

func (iodr *inodeDataReader) WriteTo(w io.Writer) (n int64, err error) {
	blockSize := iodr.er.super.blockSize()

	for _, extent := range iodr.extents {
		//fmt.Printf("extent: %+v\n", extent)
		if iodr.offset >= iodr.length { // file exhausted, should never happen
			err = io.EOF
			return
		}

		extentStartOffset := int64(extent.Block) * blockSize
		extentEndOffset := int64(extent.Block+uint32(extent.Len)) * blockSize
		if iodr.offset < extentStartOffset ||
			iodr.offset >= extentEndOffset {
			continue
		}

		chunkSize := extentEndOffset - iodr.offset
		leftInNode := iodr.length - iodr.offset
		if int64(chunkSize) > leftInNode {
			chunkSize = leftInNode
		}

		iodr.er.s.Seek(iodr.er.blockOffset(extent.Start())+
			iodr.offset-extentStartOffset, 0)
		// io.Copy* has a fixed buffer size of 32k, resulting in overly chatty HTTP
		if chunkSize < 1024*1024*4 {
			buf := make([]byte, chunkSize)
			for bn := 0; bn < len(buf) && err == nil; {
				nn, nerr := iodr.er.s.Read(buf[bn:])
				err = nerr
				bn += nn
				iodr.offset += int64(nn)
				n += int64(nn)
				if err != nil {
					return n, err
				}
			}
			for bn := 0; bn < len(buf) && err == nil; {
				nn, nerr := w.Write(buf)
				err = nerr
				bn += nn
				if err != nil {
					return n, err
				}
			}
		} else {
			nn, err := io.CopyN(w, iodr.er.s, chunkSize)
			iodr.offset += int64(nn)
			n += nn
			if err != nil {
				return n, err
			}
		}
	}
	return n, nil
}

type Inode struct {
	// File mode. Any of:
	//0x1	S_IXOTH (Others may execute)
	//0x2	S_IWOTH (Others may write)
	//0x4	S_IROTH (Others may read)
	//0x8	S_IXGRP (Group members may execute)
	//0x10	S_IWGRP (Group members may write)
	//0x20	S_IRGRP (Group members may read)
	//0x40	S_IXUSR (Owner may execute)
	//0x80	S_IWUSR (Owner may write)
	//0x100	S_IRUSR (Owner may read)
	//0x200	S_ISVTX (Sticky bit)
	//0x400	S_ISGID (Set GID)
	//0x800	S_ISUID (Set UID)
	//These are mutually-exclusive file types:
	//0x1000	S_IFIFO (FIFO)
	//0x2000	S_IFCHR (Character device)
	//0x4000	S_IFDIR (Directory)
	//0x6000	S_IFBLK (Block device)
	//0x8000	S_IFREG (Regular file)
	//0xA000	S_IFLNK (Symbolic link)
	//0xC000	S_IFSOCK (Socket)
	Mode       uint16
	Uid        uint16     // Lower 16-bits of Owner UID.
	SizeLo     uint32     // Lower 32-bits of size in bytes.
	Atime      uint32     // Last access time, in seconds since the epoch.
	Ctime      uint32     // Last inode change time, in seconds since the epoch.
	Mtime      uint32     // Last data modification time, in seconds since the epoch.
	Dtime      uint32     // Deletion Time, in seconds since the epoch.
	Gid        uint16     // Lower 16-bits of GID.
	LinksCount uint16     // Hard link count.
	BlocksLo   uint32     // Lower 32-bits of "block" count. If the huge_file feature flag is not set on the filesystem, the file consumes i_blocks_lo 512-byte blocks on disk. If huge_file is set and EXT4_HUGE_FILE_FL is NOT set in inode.i_flags, then the file consumes i_blocks_lo + (i_blocks_hi << 32) 512-byte blocks on disk. If huge_file is set and EXT4_HUGE_FILE_FL IS set in inode.i_flags, then this file consumes (i_blocks_lo + i_blocks_hi << 32) filesystem blocks on disk.
	Flags      InodeFlags // Inode flags. Any of:

	Version uint32 // Version (High 32-bits of the i_generation field?)

	Data       [60]byte // Block map, extent tree or inline data. See the section "The Contents of inode.i_block".
	Generation uint32   // File version (for NFS).
	FileAclLo  uint32   // Lower 32-bits of extended attribute block. ACLs are of course one of many possible extended attributes; I think the name of this field is a result of the first use of extended attributes being for ACLs.
	SizeHigh   uint32   // / i_dir_acl	Upper 32-bits of file size. In ext2/3 this field was named i_dir_acl, though it was usually set to zero and never used.
	ObsoFaddr  uint32   // (Obsolete) fragment address.

	BlocksHigh  uint16 // Upper 16-bits of the block count. Please see the note attached to i_blocks_lo.
	FileAclHigh uint16 // Upper 16-bits of the extended attribute block (historically, the file ACL location). See the Extended Attributes section below.
	UidHigh     uint16 // Upper 16-bits of the Owner UID.
	GidHigh     uint16 // Upper 16-bits of the GID.
	ChecksumLo  uint16 // Lower 16-bits of the inode checksum.
	Reserved    uint16 // Unused.

	ExtraIsize  uint16 // Size of this inode - 128. Alternately, the size of the extended inode fields beyond the original ext2 inode, including this field.
	ChecksumHi  uint16 // Upper 16-bits of the inode checksum.
	CtimeExtra  uint32 // Extra change time bits. This provides sub-second precision. See Inode Timestamps section.
	MtimeExtra  uint32 // Extra modification time bits. This provides sub-second precision.
	AtimeExtra  uint32 // Extra access time bits. This provides sub-second precision.
	Crtime      uint32 // File creation time, in seconds since the epoch.
	CrtimeExtra uint32 // Extra file creation time bits. This provides sub-second precision.
	VersionHi   uint32 // Upper 32-bits for version number.
}

type InodeFlags uint32

const (
	// InodeFlagSecrm           InodeFlags = 0x1        // This file requires secure deletion (EXT4_SECRM_FL). (not implemented)
	// InodeFlagUnrm            InodeFlags = 0x2        // This file should be preserved, should undeletion be desired (EXT4_UNRM_FL). (not implemented)
	InodeFlagCompr     InodeFlags = 0x4  // File is compressed (EXT4_COMPR_FL). (not really implemented)
	InodeFlagSync      InodeFlags = 0x8  // All writes to the file must be synchronous (EXT4_SYNC_FL).
	InodeFlagImmutable InodeFlags = 0x10 // File is immutable (EXT4_IMMUTABLE_FL).
	InodeFlagAppend    InodeFlags = 0x20 // File can only be appended (EXT4_APPEND_FL).
	InodeFlagNodump    InodeFlags = 0x40 // The dump(1) utility should not dump this file (EXT4_NODUMP_FL).
	InodeFlagNoatime   InodeFlags = 0x80 // Do not update access time (EXT4_NOATIME_FL).
	// InodeFlagDirty           InodeFlags = 0x100      // Dirty compressed file (EXT4_DIRTY_FL). (not used)
	// InodeFlagComprBlk        InodeFlags = 0x200      // File has one or more compressed clusters (EXT4_COMPRBLK_FL). (not used)
	// InodeFlagNocompr         InodeFlags = 0x400      // Do not compress file (EXT4_NOCOMPR_FL). (not used)
	InodeFlagEncrypt     InodeFlags = 0x800  // Encrypted inode (EXT4_ENCRYPT_FL). This bit value previously was EXT4_ECOMPR_FL (compression error), which was never used.
	InodeFlagIndex       InodeFlags = 0x1000 // Directory has hashed indexes (EXT4_INDEX_FL).
	InodeFlagImagic      InodeFlags = 0x2000 // AFS magic directory (EXT4_IMAGIC_FL).
	InodeFlagJournalData InodeFlags = 0x4000 // File data must always be written through the journal (EXT4_JOURNAL_DATA_FL).
	// InodeFlagNotail          InodeFlags = 0x8000     // File tail should not be merged (EXT4_NOTAIL_FL). (not used by ext4)
	InodeFlagDirSync  InodeFlags = 0x10000  // All directory entry data should be written synchronously (see dirsync) (EXT4_DIRSYNC_FL).
	InodeFlagTopDir   InodeFlags = 0x20000  // Top of directory hierarchy (EXT4_TOPDIR_FL).
	InodeFlagHugeFile InodeFlags = 0x40000  // This is a huge file (EXT4_HUGE_FILE_FL).
	InodeFlagExtents  InodeFlags = 0x80000  // Inode uses extents (EXT4_EXTENTS_FL).
	InodeFlagEAInode  InodeFlags = 0x200000 // Inode used for a large extended attribute (EXT4_EA_INODE_FL).
	// InodeFlagEOFBlocks       InodeFlags = 0x400000   // This file has blocks allocated past EOF (EXT4_EOFBLOCKS_FL). (deprecated)
	// InodeFlagSnapfile        InodeFlags = 0x01000000 // Inode is a snapshot (EXT4_SNAPFILE_FL). (not in mainline)
	// InodeFlagSnapfileDeleted InodeFlags = 0x04000000 // Snapshot is being deleted (EXT4_SNAPFILE_DELETED_FL). (not in mainline)
	// InodeFlagSnapfileShrunk  InodeFlags = 0x08000000 // Snapshot shrink has completed (EXT4_SNAPFILE_SHRUNK_FL). (not in mainline)
	InodeFlagInlineData InodeFlags = 0x10000000 // Inode has inline data (EXT4_INLINE_DATA_FL).
	//InodeFlagReserved        InodeFlags = 0x80000000 // Reserved for ext4 library (EXT4_RESERVED_FL).
	// Aggregate flags:
	// User-visible flags.
	InodeFlagsUserVisibleMask InodeFlags = 0x4BDFFF
	// User-modifiable flags. Note that while EXT4_JOURNAL_DATA_FL and EXT4_EXTENTS_FL can be set with setattr, they are not in the kernel's EXT4_FL_USER_MODIFIABLE mask, since it needs to handle the setting of these flags in a special manner and they are masked out of the set of flags that are saved directly to i_flags.
	InodeFlagsUserModifiableMask InodeFlags = 0x4B80FF
)

func (f InodeFlags) String() string {
	flags := ""
	if f&InodeFlagCompr > 0 {
		flags += "Compr|"
	}
	if f&InodeFlagSync > 0 {
		flags += "Sync|"
	}
	if f&InodeFlagImmutable > 0 {
		flags += "Immutable|"
	}
	if f&InodeFlagAppend > 0 {
		flags += "Append|"
	}
	if f&InodeFlagNodump > 0 {
		flags += "Nodump|"
	}
	if f&InodeFlagNoatime > 0 {
		flags += "Noatime|"
	}
	if f&InodeFlagEncrypt > 0 {
		flags += "Encrypt|"
	}
	if f&InodeFlagIndex > 0 {
		flags += "Index|"
	}
	if f&InodeFlagImagic > 0 {
		flags += "Imagic|"
	}
	if f&InodeFlagJournalData > 0 {
		flags += "JournalData|"
	}
	if f&InodeFlagDirSync > 0 {
		flags += "DirSync|"
	}
	if f&InodeFlagTopDir > 0 {
		flags += "TopDir|"
	}
	if f&InodeFlagHugeFile > 0 {
		flags += "HugeFile|"
	}
	if f&InodeFlagExtents > 0 {
		flags += "Extents|"
	}
	if f&InodeFlagEAInode > 0 {
		flags += "EAInode|"
	}
	if f&InodeFlagInlineData > 0 {
		flags += "InlineData|"
	}
	if flags != "" {
		flags = flags[:len(flags)-1]
	}
	return fmt.Sprintf("%s(0x%08x)", flags, uint32(f))
}

func (inode Inode) Size() uint64 {
	return uint64(inode.SizeLo) + uint64(inode.SizeHigh)<<32
}

func (inode Inode) GetDataReader() io.Reader {
	return bytes.NewReader(inode.Data[:])
}
