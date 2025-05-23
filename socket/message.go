package socket

type MessageType uint8

const (
	LoginMessage MessageType = iota
	ChatMessage
	SystemMessage
	UnauthorizedMessage
	UnknownMessage
	PlayerMovementMessage
	PlayerJoinedMessage
	PlayerLeftMessage
	PingPongMessage
	SyncStateMessage
	UnitActionMessage
	ChunkRequestMessage
	ChunkDataMessage
	DisconnectMessage
)

type Message struct {
	Type  MessageType `json:"type"`
	Data  []byte      `json:"data"`
	Error string      `json:"error"`
}
