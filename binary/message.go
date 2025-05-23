package binary

import (
	"bytes"
	"encoding/binary"
	"io"
)

type MessageType uint8

type Message struct {
	Type  MessageType
	Data  []byte
	Error string
}

func EncodeRawMessage(msg Message) ([]byte, error) {
	buf := new(bytes.Buffer)

	// 1. Type (1 byte)
	if err := binary.Write(buf, binary.LittleEndian, msg.Type); err != nil {
		return nil, err
	}

	// 2. Data (4 byte length + data)
	dataLen := uint32(len(msg.Data))
	if err := binary.Write(buf, binary.LittleEndian, dataLen); err != nil {
		return nil, err
	}
	if _, err := buf.Write(msg.Data); err != nil {
		return nil, err
	}

	// 3. Error (2 byte length + string data)
	errorBytes := []byte(msg.Error)
	errorLen := uint16(len(errorBytes))
	if err := binary.Write(buf, binary.LittleEndian, errorLen); err != nil {
		return nil, err
	}
	if _, err := buf.Write(errorBytes); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func DecodeRawMessage(data []byte) (*Message, error) {
	buf := bytes.NewReader(data)

	var msgType MessageType
	if err := binary.Read(buf, binary.LittleEndian, &msgType); err != nil {
		return nil, err
	}

	var dataLen uint32
	if err := binary.Read(buf, binary.LittleEndian, &dataLen); err != nil {
		return nil, err
	}

	dataBytes := make([]byte, dataLen)
	if _, err := io.ReadFull(buf, dataBytes); err != nil {
		return nil, err
	}

	var errorLen uint16
	if err := binary.Read(buf, binary.LittleEndian, &errorLen); err != nil {
		return nil, err
	}

	errorBytes := make([]byte, errorLen)
	if _, err := io.ReadFull(buf, errorBytes); err != nil {
		return nil, err
	}

	return &Message{
		Type:  msgType,
		Data:  dataBytes, // binary data (decode etmek isteyen sonra eder)
		Error: string(errorBytes),
	}, nil
}
