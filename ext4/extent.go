package ext4

import (
	"encoding/binary"
	"fmt"
	"io"
)

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
