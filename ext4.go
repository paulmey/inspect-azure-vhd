package main

// Largely from https://ext4.wiki.kernel.org/index.php/Ext4_Disk_Layout

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	//"io/ioutil"
)

func NewExtReader(s io.ReadSeeker, p partitionEntry) (r ExtReader, err error) {
	r = ExtReader{
		s:     s,
		start: int64(p.LBAfirst * 512),
		size:  int64(p.Sectors * 512),
	}
	if p.Type != 0x83 {
		err = fmt.Errorf("Not a linux partition!")
		return
	}

	_, err = s.Seek(r.start+1024, 0)
	if err != nil {
		return
	}
	err = binary.Read(s, binary.LittleEndian, &r.super)
	if err != nil {
		return
	}

	if r.super.Magic != 0xEF53 {
		err = fmt.Errorf("This does not seem to be an ext2/3/4 partition!")
		return
	}

	r.super.Print()

	return
}

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
		if e.FileType == FileTypeDir &&
			e.Name.String() == dirname {
			inode, err := er.GetInode(e.Inode)
			if err != nil {
				return []Ext4DirEntry2{}, err
			}
			return er.traversePath(inode, path[i:])
		}
	}

	return []Ext4DirEntry2{}, ErrNotFound
}

var ErrNotFound = fmt.Errorf("Not Found")

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

func (er *ExtReader) GetGroupDescriptor(n uint32) (gd Ext4GroupDescriptor, err error) {
	_, err = er.s.Seek(er.blockOffset(1)+int64(n)*int64(er.super.gdSize()), 0)
	if err != nil {
		return
	}
	err = binary.Read(er.s, binary.LittleEndian, &gd)
	return
}

func (er *ExtReader) GetInode(n uint32) (inode Ext4Inode, err error) {
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

func (er *ExtReader) GetInodeReader(inode Ext4Inode) (io.Reader, error) {
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

func (er *ExtReader) GetInodeContent(inode Ext4Inode) ([]byte, error) {
	r, err := er.GetInodeReader(inode)
	if err != nil {
		return nil, err
	}
	b := make([]byte, inode.Size())
	_, err = r.Read(b)
	return b, err
}

type inodeDataReader struct {
	er      *ExtReader
	extents []Ext4Extent
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

func (gd Ext4GroupDescriptor) InodeTableBlock() int64 {
	return int64(gd.InodeTableLo) + int64(gd.InodeTableHi)<<32
}

type Ext4GroupDescriptor struct {
	BlockBitmapLo     uint32 // Lower 32-bits of location of block bitmap.
	InodeBitmapLo     uint32 // Lower 32-bits of location of inode bitmap.
	InodeTableLo      uint32 // Lower 32-bits of location of inode table.
	FreeBlocksCountLo uint16 // Lower 16-bits of free block count.
	FreeInodesCountLo uint16 // Lower 16-bits of free inode count.
	UsedDirsCountLo   uint16 // Lower 16-bits of directory count.
	// Block group flags. Any of:
	//0x1	inode table and bitmap are not initialized (EXT4_BG_INODE_UNINIT).
	//0x2	block bitmap is not initialized (EXT4_BG_BLOCK_UNINIT).
	//0x4	inode table is zeroed (EXT4_BG_INODE_ZEROED).
	Flags             uint16
	ExcludeBitmapLo   uint32 // Lower 32-bits of location of snapshot exclusion bitmap.
	BlockBitmapCsumLo uint16 // Lower 16-bits of the block bitmap checksum.
	InodeBitmapCsumLo uint16 // Lower 16-bits of the inode bitmap checksum.
	ItableUnusedLo    uint16 // Lower 16-bits of unused inode count. If set, we needn't scan past the (sb.s_inodes_per_group - gdt.bg_itable_unused)th entry in the inode table for this group.
	Checksum          uint16 // Group descriptor checksum; crc16(sb_uuid+group+desc) if the RO_COMPAT_GDT_CSUM feature is set, or crc32c(sb_uuid+group_desc) & 0xFFFF if the RO_COMPAT_METADATA_CSUM feature is set.
	//These fields only exist if the 64bit feature is enabled and s_desc_size > 32.
	BlockBitmapHi     uint32 // Upper 32-bits of location of block bitmap.
	InodeBitmapHi     uint32 // Upper 32-bits of location of inodes bitmap.
	InodeTableHi      uint32 // Upper 32-bits of location of inodes table.
	FreeBlocksCountHi uint16 // Upper 16-bits of free block count.
	FreeInodesCountHi uint16 // Upper 16-bits of free inode count.
	UsedDirsCountHi   uint16 // Upper 16-bits of directory count.
	ItableUnusedHi    uint16 // Upper 16-bits of unused inode count.
	ExcludeBitmapHi   uint32 // Upper 32-bits of location of snapshot exclusion bitmap.
	BlockBitmapCsumHi uint16 // Upper 16-bits of the block bitmap checksum.
	InodeBitmapCsumHi uint16 // Upper 16-bits of the inode bitmap checksum.
	_                 [4]byte
}

type Ext4Inode struct {
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

func (inode Ext4Inode) Size() uint64 {
	return uint64(inode.SizeLo) + uint64(inode.SizeHigh)<<32
}

type Ext4ExtentHeader struct {
	Magic      uint16 // Magic number, 0xF30A.
	Entries    uint16 // Number of valid entries following the header.
	Max        uint16 // Maximum number of entries that could follow the header.
	Depth      uint16 // Depth of this extent node in the extent tree. 0 = this extent node points to data blocks; otherwise, this extent node points to other extent nodes. The extent tree can be at most 5 levels deep: a logical block number can be at most 2^32, and the smallest n that satisfies 4*(((blocksize - 12)/12)^n) >= 2^32 is 5.
	Generation uint32 // Generation of the tree. (Used by Lustre, but not standard ext4).
}

type Ext4ExtentIdx struct {
	Block  uint32 // This index node covers file blocks from 'block' onward.
	LeafLo uint32 // Lower 32-bits of the block number of the extent node that is the next level lower in the tree. The tree node pointed to can be either another internal node or a leaf node, described below.
	LeafHi uint16 // Upper 16-bits of the previous field.
	_      [2]byte
}

func (idx Ext4ExtentIdx) Leaf() int64 {
	return int64(idx.LeafHi)<<32 + int64(idx.LeafLo)
}

type Ext4Extent struct {
	Block   uint32 // First file block number that this extent covers.
	Len     uint16 // Number of blocks covered by extent. If the value of this field is <= 32768, the extent is initialized. If the value of the field is > 32768, the extent is uninitialized and the actual extent length is ee_len - 32768. Therefore, the maximum length of a initialized extent is 32768 blocks, and the maximum length of an uninitialized extent is 32767.
	StartHi uint16 // Upper 16-bits of the block number to which this extent points.
	StartLo uint32 // Lower 32-bits of the block number to which this extent points.
}

func (e Ext4Extent) Start() int64 {
	return int64(e.StartHi)<<32 + int64(e.StartLo)
}

func (inode Ext4Inode) GetDataReader() io.Reader {
	return bytes.NewReader(inode.Data[:])
}

func (er ExtReader) GetExtents(inode Ext4Inode) ([]Ext4Extent, error) {
	if inode.Flags&InodeFlagExtents == 0 {
		return nil, errNotImplemented
	}

	inodeData := inode.GetDataReader()
	return er.readExtents(inodeData)
}

func (er ExtReader) readExtents(r io.Reader) ([]Ext4Extent, error) {
	var eh Ext4ExtentHeader
	err := binary.Read(r, binary.LittleEndian, &eh)
	if err != nil {
		return nil, err
	}
	if eh.Magic != 0xF30A {
		return nil, fmt.Errorf("Extent header magic did not match 0x%X!=0xF30A", eh.Magic)
	}

	//fmt.Printf("  Extent header: %+v\n", eh)
	if eh.Depth == 0 { // leaf nodes
		extents := make([]Ext4Extent, eh.Entries, eh.Entries)
		for i := uint16(0); i < eh.Entries; i++ {
			err = binary.Read(r, binary.LittleEndian, &extents[i])
			if err != nil {
				return nil, err
			}
		}
		return extents, nil
	} else {
		extentIndexes := make([]Ext4ExtentIdx, eh.Entries, eh.Entries)
		for i := uint16(0); i < eh.Entries; i++ {
			err = binary.Read(r, binary.LittleEndian, &extentIndexes[i])
			if err != nil {
				return nil, err
			}
		}

		extents := []Ext4Extent{}
		for _, idx := range extentIndexes {
			er.s.Seek(er.blockOffset(idx.Leaf()), 0)
			subextents, err := er.readExtents(er.s)
			if err != nil {
				return nil, err
			}
			extents = append(extents, subextents...)
		}
		return extents, nil
	}
}

type ExtReader struct {
	s     io.ReadSeeker
	start int64
	size  int64
	super Ext4SuperBlock
}

func (r ExtReader) blockOffset(blockNo int64) int64 {
	//fmt.Printf("[[ ?? block %d ?? ]]\n", blockNo)
	return r.start + blockNo*r.super.blockSize()
}

func (s Ext4SuperBlock) gdSize() uint32 {
	if s.FeatureIncompat&FeatureIncompatFlag64Bit > 0 {
		return 64
	}
	return 32
}

func (s Ext4SuperBlock) blockSize() int64 {
	return 1 << (10 + s.LogBlockSize)
}

type Ext4SuperBlock struct {
	InodesCount   uint32 // Total inode count.
	BlocksCountLo uint32 // Total block count.

	_ [16]byte

	LogBlockSize     uint32 // Block size is 2 ^ (10 + s_log_block_size).
	LogClusterSize   uint32 // Cluster size is (2 ^ s_log_cluster_size) blocks if bigalloc is enabled, zero otherwise.
	BlocksPerGroup   uint32 // Blocks per group.
	ClustersPerGroup uint32 // Clusters per group, if bigalloc is enabled.
	InodesPerGroup   uint32 // Inodes per group.

	_ [12]byte

	Magic uint16           // Magic signature, 0xEF53
	State Ext4FSStateFlags // File system state. Valid values are

	_ [16]byte

	RevisionLevel uint32 // Revision level. One of: 0 Original format, 1 v2 format w/ dynamic inode sizes

	_ [8]byte

	InodeSize       uint16               // Size of inode structure, in bytes.
	BlockGroupNr    uint16               // Block group # of this superblock.
	FeatureCompat   FeatureCompatFlags   // Compatible feature set flags. Kernel can still read/write this fs even if it doesn't understand a flag; fsck should not do that. Any of:
	FeatureIncompat FeatureIncompatFlags // Incompatible feature set. If the kernel or fsck doesn't understand one of these bits, it should stop. Any of:
	FeatureROCompat FeatureROCompatFlags // Readonly-compatible feature set. If the kernel doesn't understand one of these bits, it can still mount read-only. Any of:
	UUID            UUID                 // 128-bit UUID for volume.
	VolumeName      [16]byte             // Volume label.

	_ [200]byte

	//64bit support valid if EXT4_FEATURE_COMPAT_64BIT
	BlocksCountHi uint32 // High 32-bits of the block count.

	_ [684]byte
}

func (s *Ext4SuperBlock) Print() {
	//fmt.Printf("RAW:             %+v\n", s)

	fmt.Printf("Inode count:     %v\n", s.InodesCount)
	fmt.Printf("Block count:     %v\n", uint64(s.BlocksCountLo)+uint64(s.BlocksCountLo)<<32)

	fmt.Printf("Block size:      %v\n", s.blockSize())
	fmt.Printf("Cluster size:    %v\n", 1<<s.LogClusterSize)

	fmt.Printf("Blocks/group:    %v\n", s.BlocksPerGroup)
	fmt.Printf("Clusters/group:  %v\n", s.ClustersPerGroup)
	fmt.Printf("Inode/group:     %v\n", s.InodesPerGroup)

	fmt.Printf("Magic:           %v\n", s.Magic)
	fmt.Printf("State:           %v\n", s.State)

	fmt.Printf("FeatureCompat:   %v\n", s.FeatureCompat)
	fmt.Printf("FeatureIncompat: %v\n", s.FeatureIncompat)
	fmt.Printf("FeatureROCompat: %v\n", s.FeatureROCompat)
}

type UUID [16]byte

func (b UUID) String() string {
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}

type FeatureCompatFlags uint32

const (
	FeatureCompatFlagDirPrealloc   FeatureCompatFlags = 0x1   // Directory preallocation (COMPAT_DIR_PREALLOC).
	FeatureCompatFlagImagicInodes  FeatureCompatFlags = 0x2   // "imagic inodes". Not clear from the code what this does (COMPAT_IMAGIC_INODES).
	FeatureCompatFlagHasJournal    FeatureCompatFlags = 0x4   // Has a journal (COMPAT_HAS_JOURNAL).
	FeatureCompatFlagExtAttr       FeatureCompatFlags = 0x8   // Supports extended attributes (COMPAT_EXT_ATTR).
	FeatureCompatFlagResizeInodes  FeatureCompatFlags = 0x10  // Has reserved GDT blocks for filesystem expansion (COMPAT_RESIZE_INODE).
	FeatureCompatFlagDirIndex      FeatureCompatFlags = 0x20  // Has directory indices (COMPAT_DIR_INDEX).
	FeatureCompatFlagLazyBG        FeatureCompatFlags = 0x40  // "Lazy BG". Not in Linux kernel, seems to have been for uninitialized block groups? (COMPAT_LAZY_BG)
	FeatureCompatFlagExcludeInode  FeatureCompatFlags = 0x80  // "Exclude inode". Not used. (COMPAT_EXCLUDE_INODE).
	FeatureCompatFlagExcludeBitmap FeatureCompatFlags = 0x100 // "Exclude bitmap". Seems to be used to indicate the presence of snapshot-related exclude bitmaps? Not defined in kernel or used in e2fsprogs (COMPAT_EXCLUDE_BITMAP).
	FeatureCompatFlagSparseSuper2  FeatureCompatFlags = 0x200 // Sparse Super Block, v2. If this flag is set, the SB field s_backup_bgs points to the two block groups that contain backup superblocks (COMPAT_SPARSE_SUPER2).
)

func (f FeatureCompatFlags) String() string {
	flags := ""
	if f&FeatureCompatFlagDirPrealloc > 0 {
		flags += "DirPrealloc|"
	}
	if f&FeatureCompatFlagImagicInodes > 0 {
		flags += "ImagicInodes|"
	}
	if f&FeatureCompatFlagHasJournal > 0 {
		flags += "HasJournal|"
	}
	if f&FeatureCompatFlagExtAttr > 0 {
		flags += "ExtAttr|"
	}
	if f&FeatureCompatFlagResizeInodes > 0 {
		flags += "ResizeInodes|"
	}
	if f&FeatureCompatFlagDirIndex > 0 {
		flags += "DirIndex|"
	}
	if f&FeatureCompatFlagLazyBG > 0 {
		flags += "LazyBG|"
	}
	if f&FeatureCompatFlagExcludeInode > 0 {
		flags += "ExcludeInode|"
	}
	if f&FeatureCompatFlagExcludeBitmap > 0 {
		flags += "ExcludeBitmap|"
	}
	if f&FeatureCompatFlagSparseSuper2 > 0 {
		flags += "SparseSuper2|"
	}
	if flags != "" {
		flags = flags[:len(flags)-1]
	}
	return fmt.Sprintf("%s(0x%08x)", flags, uint32(f))
}

type FeatureIncompatFlags uint32

const (
	FeatureIncompatFlagCompression   FeatureIncompatFlags = 0x1     // Compression (INCOMPAT_COMPRESSION).
	FeatureIncompatFlagFiletype      FeatureIncompatFlags = 0x2     // Directory entries record the file type. See ext4_dir_entry_2 below (INCOMPAT_FILETYPE).
	FeatureIncompatFlagRecoder       FeatureIncompatFlags = 0x4     // Filesystem needs recovery (INCOMPAT_RECOVER).
	FeatureIncompatFlagJournalDev    FeatureIncompatFlags = 0x8     // Filesystem has a separate journal device (INCOMPAT_JOURNAL_DEV).
	FeatureIncompatFlagMetaBG        FeatureIncompatFlags = 0x10    // Meta block groups. See the earlier discussion of this feature (INCOMPAT_META_BG).
	FeatureIncompatFlagExtents       FeatureIncompatFlags = 0x40    // Files in this filesystem use extents (INCOMPAT_EXTENTS).
	FeatureIncompatFlag64Bit         FeatureIncompatFlags = 0x80    // Enable a filesystem size of 2^64 blocks (INCOMPAT_64BIT).
	FeatureIncompatFlagMMP           FeatureIncompatFlags = 0x100   // Multiple mount protection. Not implemented (INCOMPAT_MMP).
	FeatureIncompatFlagFlexBG        FeatureIncompatFlags = 0x200   // Flexible block groups. See the earlier discussion of this feature (INCOMPAT_FLEX_BG).
	FeatureIncompatFlagEAInode       FeatureIncompatFlags = 0x400   // Inodes can be used for large extended attributes (INCOMPAT_EA_INODE). (Not implemented?)
	FeatureIncompatFlagDirdata       FeatureIncompatFlags = 0x1000  // Data in directory entry (INCOMPAT_DIRDATA). (Not implemented?)
	FeatureIncompatFlagBGUseMetaCsum FeatureIncompatFlags = 0x2000  // Never used (INCOMPAT_BG_USE_META_CSUM). Available for use.
	FeatureIncompatFlagLargedir      FeatureIncompatFlags = 0x4000  // Large directory >2GB or 3-level htree (INCOMPAT_LARGEDIR).
	FeatureIncompatFlagInlineData    FeatureIncompatFlags = 0x8000  // Data in inode (INCOMPAT_INLINE_DATA).
	FeatureIncompatFlagEncrypt       FeatureIncompatFlags = 0x10000 // Encrypted inodes are present on the filesystem. (INCOMPAT_ENCRYPT).
)

func (f FeatureIncompatFlags) String() string {
	flags := ""
	if f&FeatureIncompatFlagCompression > 0 {
		flags += "Compression|"
	}
	if f&FeatureIncompatFlagFiletype > 0 {
		flags += "Filetype|"
	}
	if f&FeatureIncompatFlagRecoder > 0 {
		flags += "Recoder|"
	}
	if f&FeatureIncompatFlagJournalDev > 0 {
		flags += "JournalDev|"
	}
	if f&FeatureIncompatFlagMetaBG > 0 {
		flags += "MetaBG|"
	}
	if f&FeatureIncompatFlagExtents > 0 {
		flags += "Extents|"
	}
	if f&FeatureIncompatFlag64Bit > 0 {
		flags += "64Bit|"
	}
	if f&FeatureIncompatFlagMMP > 0 {
		flags += "MMP|"
	}
	if f&FeatureIncompatFlagFlexBG > 0 {
		flags += "FlexBG|"
	}
	if f&FeatureIncompatFlagEAInode > 0 {
		flags += "EAInode|"
	}
	if f&FeatureIncompatFlagDirdata > 0 {
		flags += "Dirdata|"
	}
	if f&FeatureIncompatFlagBGUseMetaCsum > 0 {
		flags += "BGUseMetaCsum|"
	}
	if f&FeatureIncompatFlagLargedir > 0 {
		flags += "Largedir|"
	}
	if f&FeatureIncompatFlagInlineData > 0 {
		flags += "InlineData|"
	}
	if f&FeatureIncompatFlagEncrypt > 0 {
		flags += "Encrypt|"
	}
	if flags != "" {
		flags = flags[:len(flags)-1]
	}
	return fmt.Sprintf("%s(0x%08x)", flags, uint32(f))
}

type FeatureROCompatFlags uint32

const (
	FeatureROCompatFlagSparseSuper  FeatureROCompatFlags = 0x1    // Sparse superblocks. See the earlier discussion of this feature (RO_COMPAT_SPARSE_SUPER).
	FeatureROCompatFlagLargeFile    FeatureROCompatFlags = 0x2    // This filesystem has been used to store a file greater than 2GiB (RO_COMPAT_LARGE_FILE).
	FeatureROCompatFlagBtreeDir     FeatureROCompatFlags = 0x4    // Not used in kernel or e2fsprogs (RO_COMPAT_BTREE_DIR).
	FeatureROCompatFlagHugeFile     FeatureROCompatFlags = 0x8    // This filesystem has files whose sizes are represented in units of logical blocks, not 512-byte sectors. This implies a very large file indeed! (RO_COMPAT_HUGE_FILE)
	FeatureROCompatFlagGDTCsum      FeatureROCompatFlags = 0x10   // Group descriptors have checksums. In addition to detecting corruption, this is useful for lazy formatting with uninitialized groups (RO_COMPAT_GDT_CSUM).
	FeatureROCompatFlagDirNlink     FeatureROCompatFlags = 0x20   // Indicates that the old ext3 32,000 subdirectory limit no longer applies (RO_COMPAT_DIR_NLINK).
	FeatureROCompatFlagExtraIsize   FeatureROCompatFlags = 0x40   // Indicates that large inodes exist on this filesystem (RO_COMPAT_EXTRA_ISIZE).
	FeatureROCompatFlagHasSnapshot  FeatureROCompatFlags = 0x80   // This filesystem has a snapshot (RO_COMPAT_HAS_SNAPSHOT).
	FeatureROCompatFlagQuota        FeatureROCompatFlags = 0x100  // Quota (RO_COMPAT_QUOTA).
	FeatureROCompatFlagBigalloc     FeatureROCompatFlags = 0x200  // This filesystem supports "bigalloc", which means that file extents are tracked in units of clusters (of blocks) instead of blocks (RO_COMPAT_BIGALLOC).
	FeatureROCompatFlagMetadataCsum FeatureROCompatFlags = 0x400  // This filesystem supports metadata checksumming. (RO_COMPAT_METADATA_CSUM; implies RO_COMPAT_GDT_CSUM, though GDT_CSUM must not be set)
	FeatureROCompatFlagReplica      FeatureROCompatFlags = 0x800  // Filesystem supports replicas. This feature is neither in the kernel nor e2fsprogs. (RO_COMPAT_REPLICA)
	FeatureROCompatFlagReadonly     FeatureROCompatFlags = 0x1000 // Read-only filesystem image; the kernel will not mount this image read-write and most tools will refuse to write to the image. (RO_COMPAT_READONLY)
)

func (f FeatureROCompatFlags) String() string {
	flags := ""
	if f&FeatureROCompatFlagSparseSuper > 0 {
		flags += "SparseSuper|"
	}
	if f&FeatureROCompatFlagLargeFile > 0 {
		flags += "LargeFile|"
	}
	if f&FeatureROCompatFlagBtreeDir > 0 {
		flags += "BtreeDir|"
	}
	if f&FeatureROCompatFlagHugeFile > 0 {
		flags += "HugeFile|"
	}
	if f&FeatureROCompatFlagGDTCsum > 0 {
		flags += "GDTCsum|"
	}
	if f&FeatureROCompatFlagDirNlink > 0 {
		flags += "DirNlink|"
	}
	if f&FeatureROCompatFlagExtraIsize > 0 {
		flags += "ExtraIsize|"
	}
	if f&FeatureROCompatFlagHasSnapshot > 0 {
		flags += "HasSnapshot|"
	}
	if f&FeatureROCompatFlagQuota > 0 {
		flags += "Quota|"
	}
	if f&FeatureROCompatFlagBigalloc > 0 {
		flags += "Bigalloc|"
	}
	if f&FeatureROCompatFlagMetadataCsum > 0 {
		flags += "MetadataCsum|"
	}
	if f&FeatureROCompatFlagReplica > 0 {
		flags += "Replica|"
	}
	if f&FeatureROCompatFlagReadonly > 0 {
		flags += "Readonly|"
	}
	if flags != "" {
		flags = flags[:len(flags)-1]
	}
	return fmt.Sprintf("%s(0x%08x)", flags, uint32(f))
}

type Ext4FSStateFlags uint16

const (
	Ext4FSStateFlagClean   Ext4FSStateFlags = 1 << iota // Cleanly umounted
	Ext4FSStateFlagError                                // Errors detected
	Ext4FSStateFlagOrphans                              // Orphans being recovered
)
