package socket

import (
	"log"
	b "projectt/binary"
	"projectt/types"
	"time"
)

func handleMessage(server *GameServer, gc *GameConnection, data []byte) {
	// Decode and handle message
	msg, err := b.DecodeRawMessage(data)
	if err != nil {
		log.Printf("Error decoding message: %v - Packet length: %d - Data: %v", err, len(data), data)
		return
	}

	// Update last heartbeat time
	gc.mu.Lock()
	gc.lastHeartbeat = time.Now()
	gc.mu.Unlock()

	// Handle message based on type
	switch msg.Type {
	case types.LoginMessage:
		gc.handleLogin(msg.Data)
	case types.ChatMessage:
		gc.handleChat(msg.Data)
	case types.PlayerMovementMessage:
		gc.handleMovement(msg.Data)
	case types.PlayerDataMessage:
		gc.handlePlayerData(msg.Data)
	case types.ChunkRequestMessage:
		gc.handleChunkRequest(msg.Data)
	case types.DisconnectMessage:
		server.mu.Lock()
		gc.handleDisconnect()
		server.mu.Unlock()
		return
	case types.PingPongMessage:
		gc.handlePingPong(*msg)
	default:
		// unknown message
	}
}
