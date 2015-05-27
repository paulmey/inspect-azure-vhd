package ext4

import (
	"fmt"
)

func (s SuperBlock) gdSize() uint32 {
	if s.FeatureIncompat&FeatureIncompatFlag64Bit > 0 {
		if s.DescSize > 0 {
			return uint32(s.DescSize)
		}
		return 64
	}
	return 32
}

func (s SuperBlock) blockSize() int64 {
	return 1 << (10 + s.LogBlockSize)
}

type SuperBlock struct {
	InodesCount   uint32 // Total inode count.
	BlocksCountLo uint32 // Total block count.

	_ [16]byte

	LogBlockSize     uint32 // Block size is 2 ^ (10 + s_log_block_size).
	LogClusterSize   uint32 // Cluster size is (2 ^ s_log_cluster_size) blocks if bigalloc is enabled, zero otherwise.
	BlocksPerGroup   uint32 // Blocks per group.
	ClustersPerGroup uint32 // Clusters per group, if bigalloc is enabled.
	InodesPerGroup   uint32 // Inodes per group.

	_ [12]byte

	Magic uint16       // Magic signature, 0xEF53
	State FSStateFlags // File system state. Valid values are

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

	_ [118]byte

	DescSize uint16 // Size of group descriptors, in bytes, if the 64bit incompat feature flag is set.

	_ [80]byte

	//64bit support valid if EXT4_FEATURE_COMPAT_64BIT
	BlocksCountHi uint32 // High 32-bits of the block count.

	_ [684]byte
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
	FeatureIncompatFlagRecover       FeatureIncompatFlags = 0x4     // Filesystem needs recovery (INCOMPAT_RECOVER).
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
	if f&FeatureIncompatFlagRecover > 0 {
		flags += "Recover|"
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

type FSStateFlags uint16

const (
	FSStateFlagClean   FSStateFlags = 1 << iota // Cleanly umounted
	FSStateFlagError                            // Errors detected
	FSStateFlagOrphans                          // Orphans being recovered
)

func (s SuperBlock) String() string {
	//fmt.Printf("RAW:             %+v\n", s)

	rv := fmt.Sprintf("Inode count:     %v\n", s.InodesCount)
	rv += fmt.Sprintf("Block count:     %v\n", uint64(s.BlocksCountLo)+uint64(s.BlocksCountLo)<<32)

	rv += fmt.Sprintf("Block size:      %v\n", s.blockSize())
	rv += fmt.Sprintf("Cluster size:    %v\n", 1<<s.LogClusterSize)

	rv += fmt.Sprintf("Blocks/group:    %v\n", s.BlocksPerGroup)
	rv += fmt.Sprintf("Clusters/group:  %v\n", s.ClustersPerGroup)
	rv += fmt.Sprintf("Inode/group:     %v\n", s.InodesPerGroup)

	rv += fmt.Sprintf("Magic:           %v\n", s.Magic)
	rv += fmt.Sprintf("State:           %v\n", s.State)

	rv += fmt.Sprintf("FeatureCompat:   %v\n", s.FeatureCompat)
	rv += fmt.Sprintf("FeatureIncompat: %v\n", s.FeatureIncompat)
	rv += fmt.Sprintf("FeatureROCompat: %v\n", s.FeatureROCompat)

	return rv
}
