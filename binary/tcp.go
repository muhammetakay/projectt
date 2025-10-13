package binary

func EncodeMessage(msg Message) ([]byte, error) {
	return EncodeRawMessage(msg)
}

func DecodeMessage(data []byte) (*Message, error) {
	return DecodeRawMessage(data)
}
