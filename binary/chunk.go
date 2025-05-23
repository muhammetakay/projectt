package binary

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"time"
)

type ChunkTile struct {
	CountryID           uint8
	IsBorder            bool
	Type                uint8
	PrefabID            *uint16
	OccupiedByCountryID *uint8
	OccupiedAt          *time.Time // Unix timestamp
}

type ChunkPacket struct {
	ChunkX uint16
	ChunkY uint16
	Tiles  [256]ChunkTile // 16x16 tile
}

type ChunkRequest struct {
	ChunkX uint16
	ChunkY uint16
}

func EncodeChunkPacket(packet ChunkPacket) ([]byte, error) {
	buf := new(bytes.Buffer)

	// Chunk koordinatları
	if err := binary.Write(buf, binary.BigEndian, packet.ChunkX); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, packet.ChunkY); err != nil {
		return nil, err
	}

	// Tüm tile'ları sırayla yaz
	for _, tile := range packet.Tiles {
		binary.Write(buf, binary.BigEndian, tile.CountryID)
		border := uint8(0)
		if tile.IsBorder {
			border = 1
		}
		binary.Write(buf, binary.BigEndian, border)
		binary.Write(buf, binary.BigEndian, tile.Type)
		if tile.PrefabID != nil {
			if err := binary.Write(buf, binary.BigEndian, *tile.PrefabID); err != nil {
				return nil, err
			}
		} else {
			var zero uint16 = 0
			if err := binary.Write(buf, binary.BigEndian, zero); err != nil {
				return nil, err
			}
		}

		if tile.OccupiedByCountryID != nil {
			buf.WriteByte(1) // occupied
			binary.Write(buf, binary.BigEndian, uint8(*tile.OccupiedByCountryID))
		} else {
			buf.WriteByte(0)
			binary.Write(buf, binary.BigEndian, uint8(0)) // dummy
		}

		if tile.OccupiedAt != nil {
			buf.WriteByte(1) // occupied
			binary.Write(buf, binary.BigEndian, uint32(tile.OccupiedAt.Unix()))
		} else {
			buf.WriteByte(0)
			binary.Write(buf, binary.BigEndian, uint32(0)) // dummy
		}
	}

	return buf.Bytes(), nil
}

func DecodeChunkRequest(data []byte) (*ChunkRequest, error) {
	if len(data) < 4 { // minimum 2 + 2 byte
		return nil, fmt.Errorf("data too short")
	}

	buf := bytes.NewReader(data)
	m := &ChunkRequest{}

	// ChunkX (2 byte)
	if err := binary.Read(buf, binary.BigEndian, &m.ChunkX); err != nil {
		return nil, err
	}

	// ChunkY (2 byte)
	if err := binary.Read(buf, binary.BigEndian, &m.ChunkY); err != nil {
		return nil, err
	}

	return m, nil
}
