package binary

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

type ChunkTile struct {
	CountryID           uint8
	IsBorder            bool
	Type                uint8
	PrefabID            *uint16
	OccupiedByCountryID *uint8
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
	if err := binary.Write(buf, binary.LittleEndian, packet.ChunkX); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, packet.ChunkY); err != nil {
		return nil, err
	}

	// Tüm tile'ları sırayla yaz
	for _, tile := range packet.Tiles {
		binary.Write(buf, binary.LittleEndian, tile.CountryID)
		border := uint8(0)
		if tile.IsBorder {
			border = 1
		}
		binary.Write(buf, binary.LittleEndian, border)
		binary.Write(buf, binary.LittleEndian, tile.Type)
		if tile.PrefabID != nil {
			if err := binary.Write(buf, binary.LittleEndian, *tile.PrefabID); err != nil {
				return nil, err
			}
		} else {
			var zero uint16 = 0
			if err := binary.Write(buf, binary.LittleEndian, zero); err != nil {
				return nil, err
			}
		}

		if tile.OccupiedByCountryID != nil {
			binary.Write(buf, binary.LittleEndian, uint8(*tile.OccupiedByCountryID))
		} else {
			binary.Write(buf, binary.LittleEndian, uint8(0)) // dummy
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
	if err := binary.Read(buf, binary.LittleEndian, &m.ChunkX); err != nil {
		return nil, err
	}

	// ChunkY (2 byte)
	if err := binary.Read(buf, binary.LittleEndian, &m.ChunkY); err != nil {
		return nil, err
	}

	return m, nil
}
