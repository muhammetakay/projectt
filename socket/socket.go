package socket

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net"
	"os"
	b "projectt/binary"
	"projectt/config"
	"projectt/models"
	"projectt/models/request"
	"projectt/types"
	"sync"
	"time"
)

const (
	MaxConnections       = 100
	ChunkSize            = 16
	MaxViewChunkDistance = 3
	MaxViewDistance      = ChunkSize * MaxViewChunkDistance

	MaxUDPPayload = 1200
)

var messageIDCounter uint16 = 0

var (
	sentMessages     = make(map[uint16]*b.SentMessage)
	sentMessagesLock sync.RWMutex
)

func nextMessageID() uint16 {
	messageIDCounter++
	return messageIDCounter
}

type GameConnection struct {
	udpAddr *net.UDPAddr // UDP client address
	udpConn *net.UDPConn // UDP connection (shared)
	player  *models.Player
	server  *GameServer

	lastHeartbeat time.Time
}

type GameServer struct {
	connections  map[*GameConnection]bool
	countries    map[int]models.Country
	tiles        map[string]models.MapTile
	updatedTiles map[string]models.MapTile // Track tiles that need saving
	mu           sync.RWMutex
}

func NewGameServer() *GameServer {
	return &GameServer{
		connections:  make(map[*GameConnection]bool),
		countries:    make(map[int]models.Country),
		tiles:        make(map[string]models.MapTile),
		updatedTiles: make(map[string]models.MapTile),
	}
}

func NewGameConnection(addr *net.UDPAddr, conn *net.UDPConn, server *GameServer) *GameConnection {
	return &GameConnection{
		udpAddr: addr,
		udpConn: conn,
		server:  server,
	}
}

// Broadcast sends a message to all connected clients
func (s *GameServer) Broadcast(msg Message) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for conn := range s.connections {
		// Send message asynchronously to prevent blocking
		go func(c *GameConnection) {
			if err := c.SendMessage(msg); err != nil {
				log.Printf("Error broadcasting to %s: %v\n",
					c.udpAddr.String(), err)
			}
		}(conn)
	}
}

// BroadcastInRange sends a message to all clients within MaxViewDistance
func (s *GameServer) BroadcastInRange(msg Message, centerX, centerY uint16) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for conn := range s.connections {
		// Skip clients without players
		if conn.player == nil {
			continue
		}

		// Calculate distance between players
		dx := float64(conn.player.CoordX - centerX)
		dy := float64(conn.player.CoordY - centerY)
		distance := math.Sqrt(dx*dx + dy*dy)

		// Send only if within view distance
		if distance <= MaxViewDistance {
			go func(c *GameConnection) {
				if err := c.SendMessage(msg); err != nil {
					log.Printf("Error broadcasting to %s: %v\n",
						c.udpAddr.String(), err)
				}
			}(conn)
		}
	}
}

func (gc *GameConnection) handleLogin(data any) {
	// Convert data to LoginRequest
	loginData, err := json.Marshal(data)
	if err != nil {
		gc.SendMessage(Message{
			Type:  LoginMessage,
			Error: "error.invalid.request",
		})
		return
	}

	var loginRequest request.LoginRequest
	if err := json.Unmarshal(loginData, &loginRequest); err != nil {
		gc.SendMessage(Message{
			Type:  LoginMessage,
			Error: "error.invalid.request",
		})
		return
	}

	// Validate request
	if err := loginRequest.Validate(); err != nil {
		gc.SendMessage(Message{
			Type:  LoginMessage,
			Error: "error.validation",
		})
		return
	}

	// check if player with nickname already connected
	gc.server.mu.RLock()
	defer gc.server.mu.RUnlock()

	for conn := range gc.server.connections {
		if conn.player != nil && conn.player.Nickname == loginRequest.Nickname {
			gc.SendMessage(Message{
				Type:  LoginMessage,
				Error: "error.player.already_connected",
			})
			return
		}
	}

	if err := config.DB.Where("nickname ILIKE ?", loginRequest.Nickname).First(&gc.player).Error; err != nil {
		gc.SendMessage(Message{
			Type:  LoginMessage,
			Error: "error.player.not_found",
		})
		return
	}

	player, err := b.EncodePlayer(getBinaryPlayer(gc.player))
	if err != nil {
		gc.SendMessage(Message{
			Type:  LoginMessage,
			Error: "error.login.unavailable",
		})
		return
	}

	// Send success message
	gc.SendMessage(Message{
		Type: LoginMessage,
		Data: player,
	})

	// send initial data
	gc.sendSyncState()
}

func (gc *GameConnection) handleChat(data []byte) {
	if gc.player == nil {
		gc.SendMessage(Message{
			Type:  ChatMessage,
			Error: "error.login.required",
		})
		return
	}

	_, err := b.DecodeChatMessage(data)
	if err != nil {
		gc.SendMessage(Message{
			Type:  ChatMessage,
			Error: "error.chat.invalid",
		})
		return
	}

	// Broadcast chat message to all clients
	gc.server.Broadcast(Message{
		Type: ChatMessage,
		Data: data,
	})
}

func (gc *GameConnection) handleMovement(data any) {
	if gc.player == nil {
		gc.SendMessage(Message{
			Type:  UnauthorizedMessage,
			Error: "error.login.required",
		})
		return
	}

	// Convert data to PlayerMovementRequest
	moveReq, err := b.DecodePlayerMovementRequest(data.([]byte))
	if err != nil {
		return
	}

	// Calculate distance
	dx := moveReq.TargetX - gc.player.CoordX
	dy := moveReq.TargetY - gc.player.CoordY
	distance := math.Sqrt(float64(dx*dx + dy*dy))

	// Check if movement is within allowed range (e.g., 1 tile)
	if distance > 1.5 {
		gc.SendMessage(Message{
			Type: PlayerMovementMessage,
			Data: b.EncodePlayerMovementData(&b.PlayerMovementData{
				PlayerID: uint32(gc.player.ID),
				CoordX:   gc.player.CoordX,
				CoordY:   gc.player.CoordY,
			}),
		})
		return
	}

	// Find target tile
	targetTileKey := fmt.Sprintf("%d,%d", moveReq.TargetX, moveReq.TargetY)

	gc.server.mu.RLock()
	targetTile, found := gc.server.tiles[targetTileKey]
	gc.server.mu.RUnlock()

	// Check if target tile exists and is walkable
	if !found || targetTile.TileType != types.TileTypeGround {
		gc.SendMessage(Message{
			Type: PlayerMovementMessage,
			Data: b.EncodePlayerMovementData(&b.PlayerMovementData{
				PlayerID: uint32(gc.player.ID),
				CoordX:   gc.player.CoordX,
				CoordY:   gc.player.CoordY,
			}),
		})
		return
	}

	// Update player position
	gc.player.CoordX = moveReq.TargetX
	gc.player.CoordY = moveReq.TargetY

	// Notify nearby clients about the movement
	gc.server.BroadcastInRange(Message{
		Type: PlayerMovementMessage,
		Data: b.EncodePlayerMovementData(&b.PlayerMovementData{
			PlayerID: uint32(gc.player.ID),
			CoordX:   gc.player.CoordX,
			CoordY:   gc.player.CoordY,
		}),
	}, gc.player.CoordX, gc.player.CoordY)
}

func (gc *GameConnection) handleChunkRequest(data any) {
	if gc.player == nil {
		gc.SendMessage(Message{
			Type:  ChunkRequestMessage,
			Error: "error.login.required",
		})
		return
	}

	// Convert data to ChunkRequest
	chunk, err := b.DecodeChunkRequest(data.([]byte))
	if err != nil {
		gc.SendMessage(Message{
			Type:  ChunkRequestMessage,
			Error: "error.invalid.request",
		})
		return
	}

	// Calculate player's current chunk
	playerChunkX, playerChunkY := gc.player.GetChunkCoord(ChunkSize)

	gc.server.mu.RLock()
	defer gc.server.mu.RUnlock()

	// Check if requested chunk is within MaxViewChunkDistance
	chunkDx := chunk.ChunkX - playerChunkX
	chunkDy := chunk.ChunkY - playerChunkY
	chunkDistance := math.Sqrt(float64(chunkDx*chunkDx + chunkDy*chunkDy))

	if chunkDistance > MaxViewChunkDistance {
		return
	}

	// Calculate chunk boundaries
	startX := chunk.ChunkX * ChunkSize
	startY := chunk.ChunkY * ChunkSize
	endX := startX + ChunkSize
	endY := startY + ChunkSize

	chunkTiles := make([]b.ChunkTile, 0)

	// Collect tiles in this chunk
	for x := startX; x < endX; x++ {
		for y := startY; y < endY; y++ {
			if tile, exists := gc.server.tiles[fmt.Sprintf("%d,%d", x, y)]; exists {
				chunkTiles = append(chunkTiles, b.ChunkTile{
					CountryID:           uint8(tile.OwnerCountryID),
					IsBorder:            tile.IsBorder,
					Type:                uint8(tile.TileType),
					PrefabID:            tile.PrefabID,
					OccupiedByCountryID: tile.OccupiedByCountryID,
					OccupiedAt:          tile.OccupiedAt,
				})
			} else {
				chunkTiles = append(chunkTiles, b.ChunkTile{}) // empty tile
			}
		}
	}

	chunkPacket, err := b.EncodeChunkPacket(b.ChunkPacket{
		ChunkX: chunk.ChunkX,
		ChunkY: chunk.ChunkY,
		Tiles:  [256]b.ChunkTile(chunkTiles),
	})
	if err != nil {
		log.Printf("error sending chunks: %s\n", err)
		return
	}

	// Send chunk data
	gc.SendMessage(Message{
		Type: ChunkDataMessage,
		Data: chunkPacket,
	})
}

// mutex lock required before call
func (gc *GameConnection) handleDisconnect() {
	log.Printf("Disconnected: %s\n", gc.udpAddr.String())
	// If player is not nil, save it
	if gc.player != nil {
		if err := config.DB.Save(gc.player).Error; err != nil {
			log.Printf("Error saving player %s: %v\n", gc.player.Nickname, err)
		} else {
			log.Printf("Player %s saved successfully\n", gc.player.Nickname)
		}
	}
	// Delete connection
	delete(gc.server.connections, gc)
}

// func (gc *GameConnection) SendMessage(msg Message) error {
// 	data, err := json.Marshal(msg)
// 	if err != nil {
// 		return err
// 	}

// 	if _, err := gc.udpConn.WriteToUDP(data, gc.udpAddr); err != nil {
// 		log.Printf("Error sending UDP response: %v", err)
// 	}
// 	return err
// }

func (gc *GameConnection) SendMessage(msg Message) error {
	rawData, err := b.EncodeRawMessage(b.Message{
		Type:  b.MessageType(msg.Type),
		Data:  msg.Data,
		Error: msg.Error,
	})
	if err != nil {
		return err
	}

	messageID := nextMessageID()

	// Her parçaya 4 byte başlık eklenecek: [messageID:2][index:1][count:1]
	maxChunkSize := MaxUDPPayload - 4
	totalChunks := byte((len(rawData) + maxChunkSize - 1) / maxChunkSize)

	chunks := make([][]byte, 0)

	for i := byte(0); i < totalChunks; i++ {
		start := int(i) * maxChunkSize
		end := start + maxChunkSize
		if end > len(rawData) {
			end = len(rawData)
		}

		chunk := rawData[start:end]
		chunks = append(chunks, chunk)

		packet := new(bytes.Buffer)
		// Header: packetType (1 byte), messageID (2 byte), chunkIndex (1 byte), totalChunks (1 byte)
		packet.WriteByte(b.NormalPacket)
		binary.Write(packet, binary.BigEndian, messageID)
		packet.WriteByte(i)
		packet.WriteByte(totalChunks)

		packet.Write(chunk)

		if _, err := gc.udpConn.WriteToUDP(packet.Bytes(), gc.udpAddr); err != nil {
			log.Printf("Error sending UDP packet (chunk %d/%d): %v", i+1, totalChunks, err)
			return err
		}
	}

	sentMessagesLock.Lock()
	sentMessages[messageID] = &b.SentMessage{
		Chunks: chunks,
		SentAt: time.Now(),
	}
	sentMessagesLock.Unlock()

	return nil
}

func (s *GameServer) autoSaveRoutine() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		s.mu.RLock()
		activePlayers := make([]*models.Player, 0)
		updatedTiles := make([]models.MapTile, 0)

		// Collect all active players
		for conn := range s.connections {
			if conn.player != nil {
				activePlayers = append(activePlayers, conn.player)
			}
		}

		// Collect updated tiles
		for _, tile := range s.updatedTiles {
			updatedTiles = append(updatedTiles, tile)
		}

		// Clear updated tiles map after collecting
		s.updatedTiles = make(map[string]models.MapTile)
		s.mu.RUnlock()

		// Skip if nothing to save
		if len(activePlayers) == 0 && len(updatedTiles) == 0 {
			continue
		}

		// Save players in batches
		if len(activePlayers) > 0 {
			if err := config.DB.Save(&activePlayers).Error; err != nil {
				log.Printf("Error during player auto-save: %v\n", err)
			} else {
				log.Printf("Auto-saved %d players\n", len(activePlayers))
			}
		}

		// Save updated tiles in batches
		if len(updatedTiles) > 0 {
			if err := config.DB.Save(&updatedTiles).Error; err != nil {
				log.Printf("Error during tile auto-save: %v\n", err)
			} else {
				log.Printf("Auto-saved %d tiles\n", len(updatedTiles))
			}
		}
	}
}

func (s *GameServer) UpdateTile(tile models.MapTile) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := fmt.Sprintf("%d,%d", tile.CoordX, tile.CoordY)
	s.tiles[key] = tile
	s.updatedTiles[key] = tile
}

func (gc *GameConnection) sendSyncState() {
	gc.server.mu.RLock()
	defer gc.server.mu.RUnlock()

	// Collect nearby players
	nearbyPlayers := make([]*b.Player, 0)
	for conn := range gc.server.connections {
		if conn.player == nil || conn.player.ID == gc.player.ID {
			continue
		}

		dx := float64(conn.player.CoordX - gc.player.CoordX)
		dy := float64(conn.player.CoordY - gc.player.CoordY)
		distance := math.Sqrt(dx*dx + dy*dy)

		if distance <= MaxViewDistance {
			nearbyPlayers = append(nearbyPlayers, getBinaryPlayer(conn.player))
		}
	}

	// Binary countries
	binaryCountries := make([]b.Country, 0)
	for _, country := range gc.server.countries {
		binaryCountries = append(binaryCountries, getBinaryCountry(country))
	}

	// Send player movement message to nearby players for let them know
	gc.server.BroadcastInRange(Message{
		Type: PlayerMovementMessage,
		Data: b.EncodePlayerMovementData(&b.PlayerMovementData{
			PlayerID: uint32(gc.player.ID),
			CoordX:   gc.player.CoordX,
			CoordY:   gc.player.CoordY,
		}),
	}, gc.player.CoordX, gc.player.CoordY)

	data, err := b.EncodeSyncStateData(&b.SyncStateData{
		Players:     nearbyPlayers,
		Countries:   binaryCountries,
		OnlineCount: len(gc.server.connections),
	})
	if err != nil {
		return
	}

	// Send sync state message
	gc.SendMessage(Message{
		Type: SyncStateMessage,
		Data: data,
	})
}

func StartServer() {
	port := os.Getenv("APP_PORT")
	server := NewGameServer()

	// Load countries from the database
	var countries []models.Country
	if err := config.DB.Find(&countries).Error; err != nil {
		log.Fatalf("Failed to load countries: %v", err)
	}

	// Load map tiles from the database
	var tiles []models.MapTile
	if err := config.DB.Find(&tiles).Error; err != nil {
		log.Fatalf("Failed to load map tiles: %v", err)
	}

	// Store tiles in the server
	for _, tile := range tiles {
		key := fmt.Sprintf("%d,%d", tile.CoordX, tile.CoordY)
		server.tiles[key] = tile
	}

	// UDP setup
	addr, err := net.ResolveUDPAddr("udp", ":"+port)
	if err != nil {
		log.Fatalf("Failed to resolve address: %v", err)
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Fatalf("Server could not be started: %v", err)
	}
	defer conn.Close()

	fmt.Printf("UDP server is running on port %s...\n", port)

	// Start auto-save routine
	go server.autoSaveRoutine()
	// Start cleanup routine
	go server.cleanupInactiveConnections()
	// Start checking for missing packets
	go checkAndRequestMissingPackets()
	// Start cleanup old sent messages
	go cleanupOldSentMessages()

	buffer := make([]byte, 4096)
	for {
		n, remoteAddr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			log.Printf("Error reading from UDP: %v", err)
			continue
		}

		// Handle message
		go handleUDPMessage(server, conn, remoteAddr, buffer[:n])
	}
}

func handleUDPMessage(server *GameServer, conn *net.UDPConn, addr *net.UDPAddr, data []byte) {
	rawMessage, err := b.HandleIncomingPacket(data, conn, addr)
	if err != nil {
		log.Printf("Handle udp message error from %s, Error: %s\n", addr.String(), err)
		return
	}

	msg := Message{
		Type: MessageType(rawMessage.Type),
		Data: rawMessage.Data,
	}

	// Find or create game connection for this address
	gc := findOrCreateConnection(server, conn, addr)

	// Update last heartbeat time
	gc.lastHeartbeat = time.Now()

	// Handle message based on type
	switch msg.Type {
	case LoginMessage:
		gc.handleLogin(msg.Data)
	case ChatMessage:
		gc.handleChat(msg.Data)
	case PlayerMovementMessage:
		gc.handleMovement(msg.Data)
	case ChunkRequestMessage:
		gc.handleChunkRequest(msg.Data)
	case DisconnectMessage:
		server.mu.Lock()
		gc.handleDisconnect()
		server.mu.Unlock()
	default:
		// unknown message
	}
}

func findOrCreateConnection(server *GameServer, conn *net.UDPConn, addr *net.UDPAddr) *GameConnection {
	server.mu.Lock()
	defer server.mu.Unlock()

	// Look for existing connection with this address
	for gc := range server.connections {
		if gc.udpAddr.String() == addr.String() {
			return gc
		}
	}

	// Create new connection if not found
	gc := NewGameConnection(addr, conn, server)
	fmt.Printf("New connection from %s\n", addr.String())
	server.connections[gc] = true

	// Make sure we don't exceed max connections
	if len(server.connections) >= MaxConnections {
		gc.SendMessage(Message{
			Type:  SystemMessage,
			Error: "error.server.full",
		})
		gc.handleDisconnect()
		return nil
	}

	return gc
}

func (s *GameServer) cleanupInactiveConnections() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		for gc := range s.connections {
			if now.Sub(gc.lastHeartbeat) > 1*time.Minute {
				gc.handleDisconnect()
			}
		}
		s.mu.Unlock()
	}
}

func checkAndRequestMissingPackets() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		b.CheckAndRequestMissingPackets()
	}
}

func cleanupOldSentMessages() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		cutoff := time.Now().Add(-10 * time.Second)

		sentMessagesLock.Lock()
		for id, msg := range sentMessages {
			if msg.SentAt.Before(cutoff) {
				delete(sentMessages, id)
			}
		}
		sentMessagesLock.Unlock()
	}
}
