package binary

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

type Player struct {
	ID        uint32  // 4 byte
	Nickname  string  // 2 byte (Maximum 255 characters)
	CountryID uint8   // 1 byte
	EXP       uint32  // 4 byte
	Rank      byte    // 1 byte (PlayerRank enum)
	Health    uint32  // 4 byte
	MaxHealth uint32  // 4 byte
	CoordX    float32 // 4 byte
	CoordY    float32 // 4 byte
	DirX      float32 // 4 byte
	DirY      float32 // 4 byte
	UnitID    *uint   // 4 byte
}

type PlayerMovementRequest struct {
	DirX, DirY float32
	Timestamp  float32
}

type PlayerDataRequest struct {
	PlayerID uint32
}

type PlayerMovementData struct {
	PlayerID         uint32
	PosX, PosY       float32
	DirX, DirY       float32
	Speed            float32
	IsMoving         bool
	LastUpdatedTicks float32
}

func EncodePlayer(p *Player) ([]byte, error) {
	buf := new(bytes.Buffer)

	binary.Write(buf, binary.LittleEndian, uint32(p.ID))
	binary.Write(buf, binary.LittleEndian, uint8(p.CountryID))
	binary.Write(buf, binary.LittleEndian, uint32(p.EXP))
	buf.WriteByte(uint8(p.Rank))
	binary.Write(buf, binary.LittleEndian, uint32(p.Health))
	binary.Write(buf, binary.LittleEndian, uint32(p.MaxHealth))
	binary.Write(buf, binary.LittleEndian, float32(p.CoordX))
	binary.Write(buf, binary.LittleEndian, float32(p.CoordY))
	binary.Write(buf, binary.LittleEndian, float32(p.DirX))
	binary.Write(buf, binary.LittleEndian, float32(p.DirY))

	if p.UnitID != nil {
		buf.WriteByte(1) // has unit
		binary.Write(buf, binary.LittleEndian, uint32(*p.UnitID))
	} else {
		buf.WriteByte(0)
		binary.Write(buf, binary.LittleEndian, uint32(0)) // dummy
	}

	nickBytes := []byte(p.Nickname)
	nickLen := len(nickBytes)
	if nickLen > 255 {
		return nil, fmt.Errorf("nickname too long")
	}
	buf.WriteByte(uint8(nickLen))
	buf.Write(nickBytes)

	return buf.Bytes(), nil
}

func EncodePlayerMovementData(m *PlayerMovementData) []byte {
	buf := new(bytes.Buffer)

	binary.Write(buf, binary.LittleEndian, m.PlayerID)
	binary.Write(buf, binary.LittleEndian, m.PosX)
	binary.Write(buf, binary.LittleEndian, m.PosY)
	binary.Write(buf, binary.LittleEndian, m.DirX)
	binary.Write(buf, binary.LittleEndian, m.DirY)
	binary.Write(buf, binary.LittleEndian, m.Speed)
	binary.Write(buf, binary.LittleEndian, m.IsMoving)
	binary.Write(buf, binary.LittleEndian, m.LastUpdatedTicks)

	return buf.Bytes()
}

func DecodePlayerMovementRequest(data []byte) (*PlayerMovementRequest, error) {
	if len(data) < 4 { // minimum 2 + 2 byte
		return nil, fmt.Errorf("data too short")
	}

	buf := bytes.NewReader(data)
	m := &PlayerMovementRequest{}

	// DirX (4 byte)
	if err := binary.Read(buf, binary.LittleEndian, &m.DirX); err != nil {
		return nil, err
	}

	// DirY (4 byte)
	if err := binary.Read(buf, binary.LittleEndian, &m.DirY); err != nil {
		return nil, err
	}

	// Timestamp (4 byte)
	if err := binary.Read(buf, binary.LittleEndian, &m.Timestamp); err != nil {
		return nil, err
	}

	return m, nil
}

func DecodePlayerDataRequest(data []byte) (*PlayerDataRequest, error) {
	if len(data) < 4 { // minimum 2 + 2 byte
		return nil, fmt.Errorf("data too short")
	}

	buf := bytes.NewReader(data)
	m := &PlayerDataRequest{}

	// PlayerID (4 byte)
	if err := binary.Read(buf, binary.LittleEndian, &m.PlayerID); err != nil {
		return nil, err
	}

	return m, nil
}
