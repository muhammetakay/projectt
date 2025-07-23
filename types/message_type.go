package types

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
	PlayerDataMessage
	PingPongMessage
	SyncStateMessage
	UnitActionMessage
	ChunkRequestMessage
	ChunkDataMessage
	DisconnectMessage
)
