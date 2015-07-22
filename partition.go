package main

import (
	"encoding/binary"
	"io"
)

type partitionEntry struct {
	Active   byte
	CHSFirst [3]byte
	Type     byte
	CHSLast  [3]byte
	LBAfirst uint32
	Sectors  uint32
}

func readPartitionTable(s io.ReadSeeker) ([]partitionEntry, error) {
	_, err := s.Seek(446, 0)
	if err != nil {
		return nil, err
	}

	rv := make([]partitionEntry, 4)
	for i := 0; i < 4; i++ {
		err := binary.Read(s, binary.LittleEndian, &rv[i])
		if err != nil {
			return nil, err
		}
	}

	return rv, nil
}
