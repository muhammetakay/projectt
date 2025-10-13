package binary

import (
	"bytes"
	"fmt"
	"strings"
)

type ChatMessage struct {
	From    string // 2 byte (Player name, Maximum 255 characters)
	Message string // 2 byte (Maximum 255 characters)
}

func EncodeChatMessage(m *ChatMessage) ([]byte, error) {
	buf := new(bytes.Buffer)

	fromBytes := []byte(m.From)
	fromLen := len(fromBytes)
	if fromLen > 255 {
		return nil, fmt.Errorf("from name too long")
	}
	buf.WriteByte(uint8(fromLen))
	buf.Write(fromBytes)

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
	if len(data) < 1 { // minimum 1 byte for message length
		return nil, fmt.Errorf("data too short")
	}

	buf := bytes.NewReader(data)
	m := &ChatMessage{}

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
	m.Message = strings.TrimSpace(m.Message)

	return m, nil
}
