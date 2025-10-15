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
		gc.SendTCPMessage(b.Message{
			Type:  types.LoginMessage,
			Error: "error.server.full",
		})
		return
	}
	server.connections[gc.connID] = gc
	server.mu.Unlock()

	// send welcome message
	data := b.EncodeWelcomeMessage(&b.WelcomeMessage{ConnectionID: gc.connID})
	gc.SendTCPMessage(b.Message{
		Type: types.WelcomeMessage,
		Data: data,
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

		handleMessage(server, gc, data)
	}

	// Handle disconnection
	server.mu.Lock()
	gc.handleDisconnect()
	server.mu.Unlock()
}
