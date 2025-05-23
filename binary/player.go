package binary

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

type Player struct {
	ID        uint32  // 4 byte
	Nickname  string  // 2 byte (Maximum 255 characters)
	CountryID uint16  // 2 byte
	EXP       uint32  // 4 byte
	Rank      byte    // 1 byte (PlayerRank enum)
	Health    uint32  // 4 byte
	MaxHealth uint32  // 4 byte
	CoordX    uint16  // 2 byte
	CoordY    uint16  // 2 byte
	UnitID    *uint16 // 2 byte
}

type PlayerMovementRequest struct {
	TargetX uint16
	TargetY uint16
}

type PlayerMovementData struct {
	PlayerID uint32
	CoordX   uint16
	CoordY   uint16
}

func EncodePlayer(p *Player) ([]byte, error) {
	buf := new(bytes.Buffer)

	binary.Write(buf, binary.BigEndian, uint32(p.ID))
	binary.Write(buf, binary.BigEndian, uint16(p.CountryID))
	binary.Write(buf, binary.BigEndian, uint32(p.EXP))
	buf.WriteByte(uint8(p.Rank))
	binary.Write(buf, binary.BigEndian, uint32(p.Health))
	binary.Write(buf, binary.BigEndian, uint32(p.MaxHealth))
	binary.Write(buf, binary.BigEndian, uint16(p.CoordX))
	binary.Write(buf, binary.BigEndian, uint16(p.CoordY))

	if p.UnitID != nil {
		buf.WriteByte(1) // has unit
		binary.Write(buf, binary.BigEndian, uint16(*p.UnitID))
	} else {
		buf.WriteByte(0)
		binary.Write(buf, binary.BigEndian, uint16(0)) // dummy
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

	binary.Write(buf, binary.BigEndian, m.PlayerID)
	binary.Write(buf, binary.BigEndian, m.CoordX)
	binary.Write(buf, binary.BigEndian, m.CoordY)

	return buf.Bytes()
}

func DecodePlayerMovementRequest(data []byte) (*PlayerMovementRequest, error) {
	if len(data) < 4 { // minimum 2 + 2 byte
		return nil, fmt.Errorf("data too short")
	}

	buf := bytes.NewReader(data)
	m := &PlayerMovementRequest{}

	// TargetX (2 byte)
	if err := binary.Read(buf, binary.BigEndian, &m.TargetX); err != nil {
		return nil, err
	}

	// TargetY (2 byte)
	if err := binary.Read(buf, binary.BigEndian, &m.TargetY); err != nil {
		return nil, err
	}

	return m, nil
}
