package binary

import (
	"bytes"
	"encoding/binary"
)

type WelcomeMessage struct {
	ConnectionID uint32
}

func EncodeWelcomeMessage(m *WelcomeMessage) []byte {
	buf := new(bytes.Buffer)

	binary.Write(buf, binary.LittleEndian, m.ConnectionID)

	return buf.Bytes()
}
