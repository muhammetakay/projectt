package binary

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

type LoginRequest struct {
	Nickname string
}

func (r *LoginRequest) Validate() error {
	if r.Nickname == "" {
		return fmt.Errorf("error.validation.nickname.required")
	}
	return nil
}

func DecodeLoginMessage(data []byte) (*LoginRequest, error) {
	if len(data) < 1 { // minimum 1 byte
		return nil, fmt.Errorf("data too short")
	}

	buf := bytes.NewReader(data)
	m := &LoginRequest{}

	var nickLen byte
	if err := binary.Read(buf, binary.LittleEndian, &nickLen); err != nil {
		return nil, err
	}

	nickBytes := make([]byte, nickLen)
	if _, err := io.ReadFull(buf, nickBytes); err != nil {
		return nil, err
	}
	m.Nickname = string(nickBytes)

	return m, nil
}
