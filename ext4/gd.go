package ext4

import (
	"encoding/binary"
)

func (er *Reader) GetGroupDescriptor(n uint32) (gd GroupDescriptor, err error) {
	_, err = er.s.Seek(er.blockOffset(1)+int64(n)*int64(er.super.gdSize()), 0)
	if err != nil {
		return
	}
	err = binary.Read(er.s, binary.LittleEndian, &gd)

	if er.super.FeatureIncompat&FeatureIncompatFlag64Bit == 0 {
		gd.InodeTableHi = 0
		gd.BlockBitmapHi = 0
		gd.InodeBitmapHi = 0
		gd.ItableUnusedHi = 0
		gd.ExcludeBitmapHi = 0
		gd.UsedDirsCountHi = 0
		gd.BlockBitmapCsumHi = 0
		gd.FreeBlocksCountHi = 0
		gd.FreeInodesCountHi = 0
		gd.InodeBitmapCsumHi = 0
	}

	return
}

type GroupDescriptor struct {
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

func (gd GroupDescriptor) InodeTableBlock() int64 {
	return int64(gd.InodeTableLo) + int64(gd.InodeTableHi)<<32
}
