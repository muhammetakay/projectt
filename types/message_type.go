package types

type MessageType uint8

const (
	WelcomeMessage MessageType = iota
	LoginMessage
	ChatMessage
	SystemMessage
	UnauthorizedMessage
	UnknownMessage
	PlayerMovementMessage
	PlayerJoinedMessage
	PlayerLeftMessage
	PlayerDataMessage
	PingPongMessage
	SyncStateMessage
	UnitActionMessage
	ChunkRequestMessage
	ChunkDataMessage
	DisconnectMessage
)
