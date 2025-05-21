package socket

type MessageType int

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
)

type Message struct {
	Type  MessageType `json:"type"`
	Data  any         `json:"data"`
	Error string      `json:"error"`
}
