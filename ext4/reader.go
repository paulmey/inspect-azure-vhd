package ext4

import (
	"encoding/binary"
	"fmt"
	"io"
)

func NewExtReader(s io.ReadSeeker, p PartitionEntry) (r ExtReader, err error) {
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

	unsupported := r.super.FeatureIncompat &
		^(FeatureIncompatFlagFiletype |
			FeatureIncompatFlagExtents |
			FeatureIncompatFlag64Bit |
			FeatureIncompatFlagFlexBG |
			FeatureIncompatFlagRecover)

	if unsupported > 0 {
		err = fmt.Errorf("Unsupported features: %s\n", unsupported)
		return
	}

	// fixup for non-64 bit
	if r.super.FeatureIncompat|FeatureIncompatFlag64Bit == 0 {
		r.super.BlocksCountHi = 0
	}

	fmt.Printf(r.super.String())

	return
}

func (r ExtReader) blockOffset(blockNo int64) int64 {
	//fmt.Printf("[[ ?? block %d ?? ]]\n", blockNo)
	return r.start + blockNo*r.super.blockSize()
}

type ExtReader struct {
	s     io.ReadSeeker
	start int64
	size  int64
	super SuperBlock
}
