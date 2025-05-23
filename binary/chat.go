package binary

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

type ChatMessageType uint8

const (
	ChatMessageTypeGeneral ChatMessageType = iota
	ChatMessageTypeCountry
)

type ChatMessage struct {
	Type    ChatMessageType // 1 byte
	From    uint32          // 4 byte (Player ID)
	Message string          // 2 byte (Maximum 255 characters)
}

func EncodeChatMessage(m *ChatMessage) ([]byte, error) {
	buf := new(bytes.Buffer)

	buf.WriteByte(uint8(m.Type))
	binary.Write(buf, binary.BigEndian, m.From)

	messageBytes := []byte(m.Message)
	messageLen := len(messageBytes)
	if messageLen > 255 {
		return nil, fmt.Errorf("message too long")
	}
	buf.WriteByte(uint8(messageLen))
	buf.Write(messageBytes)

	return buf.Bytes(), nil
}

func DecodeChatMessage(data []byte) (*ChatMessage, error) {
	if len(data) < 6 { // minimum 1 + 4 + 1 byte
		return nil, fmt.Errorf("data too short")
	}

	buf := bytes.NewReader(data)
	m := &ChatMessage{}

	// Type (1 byte)
	t, err := buf.ReadByte()
	if err != nil {
		return nil, err
	}
	m.Type = ChatMessageType(t)

	// From (4 byte)
	if err := binary.Read(buf, binary.BigEndian, &m.From); err != nil {
		return nil, err
	}

	// Message length (1 byte)
	msgLenByte, err := buf.ReadByte()
	if err != nil {
		return nil, err
	}
	msgLen := int(msgLenByte)

	// Check if remaining data is enough
	if buf.Len() < msgLen {
		return nil, fmt.Errorf("invalid message length")
	}

	// Message (msgLen byte)
	msgBytes := make([]byte, msgLen)
	if _, err := buf.Read(msgBytes); err != nil {
		return nil, err
	}
	m.Message = string(msgBytes)

	return m, nil
}
