package socket

type MessageType string

const (
	LoginMessage        MessageType = "login"
	ChatMessage         MessageType = "chat"
	SystemMessage       MessageType = "system"
	UnauthorizedMessage MessageType = "unauthorized"
	UnknownMessage      MessageType = "unknown"
)

type Message struct {
	Type MessageType `json:"type"`
	Data any         `json:"data"`
}
