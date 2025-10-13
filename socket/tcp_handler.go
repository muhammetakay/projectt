package socket

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	b "projectt/binary"
	"projectt/config"
	"projectt/types"
	"time"
)

func handleTCPConnection(server *GameServer, conn net.Conn) {
	defer conn.Close()

	conn.(*net.TCPConn).SetNoDelay(true)

	gc := NewGameConnection(conn, server)
	fmt.Printf("New connection from %s\n", conn.RemoteAddr())

	// Make sure we don't exceed max connections
	server.mu.Lock()
	if len(server.connections) >= config.MaxPlayers {
		server.mu.Unlock()
		gc.SendMessage(b.Message{
			Type:  types.LoginMessage,
			Error: "error.server.full",
		})
		return
	}
	server.connections[gc] = true
	server.mu.Unlock()

	// send welcome message
	gc.SendMessage(b.Message{
		Type: types.SystemMessage,
		Data: []byte("Welcome to Project T!"),
	})

	// Read messages in a loop
	for {
		// Read message length (4 bytes)
		lenBuf := make([]byte, 4)
		_, err := io.ReadFull(conn, lenBuf)
		if err != nil {
			if err == io.EOF {
				break // Connection closed
			}
			log.Printf("Error reading message length: %v", err)
			break
		}
		messageLen := binary.LittleEndian.Uint32(lenBuf)

		// Read message data
		data := make([]byte, messageLen)
		_, err = io.ReadFull(conn, data)
		if err != nil {
			log.Printf("Error reading message data: %v", err)
			break
		}

		// Decode and handle message
		msg, err := b.DecodeRawMessage(data)
		if err != nil {
			log.Printf("Error decoding message: %v", err)
			continue
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

	// Handle disconnection
	server.mu.Lock()
	gc.handleDisconnect()
	server.mu.Unlock()
}
