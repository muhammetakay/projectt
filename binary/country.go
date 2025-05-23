package binary

import (
	"bytes"
	"encoding/binary"
)

type Country struct {
	ID             uint8
	Name           string
	Code           string
	IsAIControlled bool
}

func EncodeCountry(m *Country) []byte {
	buf := new(bytes.Buffer)

	buf.WriteByte(m.ID)

	nameBytes := []byte(m.Name)
	nameLen := len(nameBytes)
	buf.WriteByte(uint8(nameLen))
	binary.Write(buf, binary.LittleEndian, nameBytes)

	codeBytes := []byte(m.Code)
	codeLen := len(codeBytes)
	buf.WriteByte(uint8(codeLen))
	binary.Write(buf, binary.LittleEndian, codeBytes)

	isAiControlled := uint8(0)
	if m.IsAIControlled {
		isAiControlled = 1
	}
	buf.WriteByte(isAiControlled)

	return buf.Bytes()
}
