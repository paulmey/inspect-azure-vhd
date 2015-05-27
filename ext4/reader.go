package ext4

import (
	"encoding/binary"
	"fmt"
	"io"
)

func NewReader(s io.ReadSeeker, startBlock, blockCount uint32) (r Reader, err error) {
	r = Reader{
		s:     s,
		start: int64(startBlock * 512),
		size:  int64(blockCount * 512),
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

func (r Reader) blockOffset(blockNo int64) int64 {
	//fmt.Printf("[[ ?? block %d ?? ]]\n", blockNo)
	return r.start + blockNo*r.super.blockSize()
}

type Reader struct {
	s     io.ReadSeeker
	start int64
	size  int64
	super SuperBlock
}
