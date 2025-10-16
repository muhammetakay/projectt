package socket

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"
)

func handleUDPConnection(server *GameServer, conn *net.UDPConn, addr *net.UDPAddr, data []byte) {
	if len(data) < 4 {
		return
	}

	connID := binary.LittleEndian.Uint32(data[0:])

	// Find game connection for this address
	gc, err := findConnection(server, conn, addr, connID)
	if err != nil {
		return
	}

	handleMessage(server, gc, data[4:])
}

func findConnection(server *GameServer, conn *net.UDPConn, addr *net.UDPAddr, connID uint32) (*GameConnection, error) {
	// Look for existing connection with this address
	server.mu.RLock()
	gc, exists := server.connections[connID]
	server.mu.RUnlock()

	if exists {
		gc.mu.Lock()
		defer gc.mu.Unlock()
		if gc.udpAddr == nil {
			// First time seeing this address, bind it
			gc.udpAddr = addr
			gc.udpConn = conn
			log.Printf("Bound UDP for %s to connID %d", addr.String(), connID)
			return gc, nil
		}
		if gc.udpAddr.String() != addr.String() {
			return nil, fmt.Errorf("connection ID mismatch")
		}
		return gc, nil
	}

	return nil, fmt.Errorf("connection not found")
}
