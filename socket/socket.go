package socket

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"net"
	"os"
	b "projectt/binary"
	"projectt/config"
	"projectt/models"
	"projectt/types"
	"slices"
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

var (
	messageIDCounter   uint32 = 0
	messageIDCounterMu sync.RWMutex
)

var (
	sentMessages     = make(map[uint32]*b.SentMessage)
	sentMessagesLock sync.RWMutex
)

func nextMessageID() uint32 {
	messageIDCounterMu.Lock()
	defer messageIDCounterMu.Unlock()
	messageIDCounter++
	return messageIDCounter
}

type GameConnection struct {
	udpAddr *net.UDPAddr // UDP client address
	udpConn *net.UDPConn // UDP connection (shared)
	player  *models.Player
	server  *GameServer
	mu      sync.RWMutex // Mutex for thread-safe access

	lastHeartbeat time.Time
}

type GameServer struct {
	connections  map[*GameConnection]bool
	countries    map[uint8]models.Country
	tiles        map[string]models.MapTile
	updatedTiles map[string]models.MapTile // Track tiles that need saving
	mu           sync.RWMutex
}

func NewGameServer() *GameServer {
	return &GameServer{
		connections:  make(map[*GameConnection]bool),
		countries:    make(map[uint8]models.Country),
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
func (s *GameServer) Broadcast(msg b.Message) {
	s.mu.RLock()
	connectionsCopy := make([]*GameConnection, 0, len(s.connections))
	for conn := range s.connections {
		connectionsCopy = append(connectionsCopy, conn)
	}
	s.mu.RUnlock()

	s.broadcastInternal(msg, connectionsCopy)
}

// broadcastInternal performs the actual broadcast without acquiring server locks
func (s *GameServer) broadcastInternal(msg b.Message, connections []*GameConnection) {
	for _, conn := range connections {
		// Send message asynchronously to prevent blocking
		go func(c *GameConnection) {
			if c == nil {
				return
			}
			if err := c.SendMessage(msg); err != nil {
				log.Printf("Error broadcasting to %s: %v\n",
					c.udpAddr.String(), err)
			}
		}(conn)
	}
}
func (s *GameServer) BroadcastWithLock(msg b.Message) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	s.Broadcast(msg)
}

// BroadcastInRange sends a message to all clients within MaxViewDistance
func (s *GameServer) BroadcastInRange(msg b.Message, centerX, centerY uint16) {
	s.mu.RLock()
	connectionsCopy := make([]*GameConnection, 0, len(s.connections))
	for conn := range s.connections {
		connectionsCopy = append(connectionsCopy, conn)
	}
	s.mu.RUnlock()

	s.broadcastInRangeInternal(msg, centerX, centerY, connectionsCopy)
}

// broadcastInRangeInternal performs the actual broadcast without acquiring server locks
// This is useful when the caller already holds the server lock
func (s *GameServer) broadcastInRangeInternal(msg b.Message, centerX, centerY uint16, connections []*GameConnection) {
	for _, conn := range connections {
		go func(c *GameConnection) {
			if c == nil {
				return
			}

			c.mu.RLock()
			// Skip clients without players
			if c.player == nil {
				c.mu.RUnlock()
				return
			}

			// Calculate distance between players
			dx := float64(c.player.CoordX - centerX)
			dy := float64(c.player.CoordY - centerY)
			distance := math.Sqrt(dx*dx + dy*dy)
			playerWithinRange := distance <= MaxViewDistance
			c.mu.RUnlock()

			// Send only if within view distance
			if playerWithinRange {
				if err := c.SendMessage(msg); err != nil {
					log.Printf("Error broadcasting to %s: %v\n",
						c.udpAddr.String(), err)
				}
			}
		}(conn)
	}
}
func (s *GameServer) BroadcastInRangeWithLock(msg b.Message, centerX, centerY uint16) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	s.BroadcastInRange(msg, centerX, centerY)
}

func (gc *GameConnection) handleLogin(data []byte) {
	loginRequest, err := b.DecodeLoginMessage(data)
	if err != nil {
		gc.SendMessage(b.Message{
			Type:  types.LoginMessage,
			Error: "error.request.invalid",
		})
		return
	}

	// Validate request
	if err := loginRequest.Validate(); err != nil {
		gc.SendMessage(b.Message{
			Type:  types.LoginMessage,
			Error: err.Error(),
		})
		return
	}

	// check if player with nickname already connected
	gc.server.mu.RLock()
	connectedNicknames := make(map[string]bool)
	for conn := range gc.server.connections {
		conn.mu.RLock()
		if conn.player != nil {
			connectedNicknames[conn.player.Nickname] = true
		}
		conn.mu.RUnlock()
	}
	gc.server.mu.RUnlock()

	if connectedNicknames[loginRequest.Nickname] {
		gc.SendMessage(b.Message{
			Type:  types.LoginMessage,
			Error: "error.player.already_connected",
		})
		return
	}

	gc.mu.Lock()
	if err := config.DB.Where("nickname ILIKE ?", loginRequest.Nickname).First(&gc.player).Error; err != nil {
		gc.player = nil // reset player if not found
		gc.mu.Unlock()
		gc.SendMessage(b.Message{
			Type:  types.LoginMessage,
			Error: "error.player.not_found",
		})
		return
	}
	gc.mu.Unlock()

	binaryPlayer := getBinaryPlayer(gc.player)
	player, err := b.EncodePlayer(binaryPlayer)
	if err != nil {
		gc.SendMessage(b.Message{
			Type:  types.LoginMessage,
			Error: "error.login.unavailable",
		})
		return
	}

	// Send success message
	gc.SendMessage(b.Message{
		Type: types.LoginMessage,
		Data: player,
	})

	// send player joined message to nearby players
	gc.server.BroadcastInRange(b.Message{
		Type: types.PlayerJoinedMessage,
		Data: player,
	}, gc.player.CoordX, gc.player.CoordY)

	// send initial data
	gc.sendSyncState()
}

func (gc *GameConnection) handleChat(data []byte) {
	gc.mu.RLock()
	defer gc.mu.RUnlock()
	if gc.player == nil {
		gc.SendMessage(b.Message{
			Type:  types.ChatMessage,
			Error: "error.login.required",
		})
		return
	}

	_, err := b.DecodeChatMessage(data)
	if err != nil {
		gc.SendMessage(b.Message{
			Type:  types.ChatMessage,
			Error: "error.chat.invalid",
		})
		return
	}

	// Broadcast chat message to all clients
	gc.server.Broadcast(b.Message{
		Type: types.ChatMessage,
		Data: data,
	})
}

func (gc *GameConnection) handleMovement(data any) {
	gc.mu.Lock()
	if gc.player == nil {
		gc.mu.Unlock()
		gc.SendMessage(b.Message{
			Type:  types.UnauthorizedMessage,
			Error: "error.login.required",
		})
		return
	}

	// Convert data to PlayerMovementRequest
	moveReq, err := b.DecodePlayerMovementRequest(data.([]byte))
	if err != nil {
		gc.mu.Unlock()
		return
	}

	// Calculate distance
	dx := moveReq.TargetX - gc.player.CoordX
	dy := moveReq.TargetY - gc.player.CoordY
	distance := math.Sqrt(float64(dx*dx + dy*dy))

	// Check if movement is within allowed range (e.g., 1 tile)
	if distance > 1.5 {
		playerMovementData := b.EncodePlayerMovementData(&b.PlayerMovementData{
			PlayerID: uint32(gc.player.ID),
			CoordX:   gc.player.CoordX,
			CoordY:   gc.player.CoordY,
		})
		gc.mu.Unlock()
		gc.SendMessage(b.Message{
			Type: types.PlayerMovementMessage,
			Data: playerMovementData,
		})
		return
	}

	// Find target tile
	targetTileKey := fmt.Sprintf("%d,%d", moveReq.TargetX, moveReq.TargetY)
	gc.mu.Unlock()

	gc.server.mu.RLock()
	targetTile, found := gc.server.tiles[targetTileKey]
	gc.server.mu.RUnlock()

	// Check if target tile exists and is walkable
	if !found || targetTile.TileType != types.TileTypeGround {
		gc.mu.RLock()
		playerMovementData := b.EncodePlayerMovementData(&b.PlayerMovementData{
			PlayerID: uint32(gc.player.ID),
			CoordX:   gc.player.CoordX,
			CoordY:   gc.player.CoordY,
		})
		gc.mu.RUnlock()
		gc.SendMessage(b.Message{
			Type: types.PlayerMovementMessage,
			Data: playerMovementData,
		})
		return
	}

	// Update player position
	gc.mu.Lock()
	gc.player.CoordX = moveReq.TargetX
	gc.player.CoordY = moveReq.TargetY
	playerMovementData := b.EncodePlayerMovementData(&b.PlayerMovementData{
		PlayerID: uint32(gc.player.ID),
		CoordX:   gc.player.CoordX,
		CoordY:   gc.player.CoordY,
	})
	coordX := gc.player.CoordX
	coordY := gc.player.CoordY
	gc.mu.Unlock()

	// Notify nearby clients about the movement
	gc.server.BroadcastInRange(b.Message{
		Type: types.PlayerMovementMessage,
		Data: playerMovementData,
	}, coordX, coordY)
}

func (gc *GameConnection) handlePlayerData(data any) {
	gc.mu.RLock()
	if gc.player == nil {
		gc.mu.RUnlock()
		gc.SendMessage(b.Message{
			Type:  types.UnauthorizedMessage,
			Error: "error.login.required",
		})
		return
	}
	playerCoords := [2]uint16{gc.player.CoordX, gc.player.CoordY}
	gc.mu.RUnlock()

	// Convert data to PlayerDataRequest
	req, err := b.DecodePlayerDataRequest(data.([]byte))
	if err != nil {
		return
	}

	gc.server.mu.RLock()
	connectionsCopy := make([]*GameConnection, 0, len(gc.server.connections))
	for conn := range gc.server.connections {
		connectionsCopy = append(connectionsCopy, conn)
	}
	gc.server.mu.RUnlock()

	// Check if player exists
	var player *models.Player
	for _, conn := range connectionsCopy {
		if conn == nil {
			continue
		}
		conn.mu.RLock()
		if conn.player != nil && conn.player.ID == uint(req.PlayerID) {
			// Create a copy of the player data to avoid holding the lock
			player = &models.Player{
				Model: models.Model{
					ID: conn.player.ID,
				},
				CoordX:    conn.player.CoordX,
				CoordY:    conn.player.CoordY,
				Nickname:  conn.player.Nickname,
				CountryID: conn.player.CountryID,
				EXP:       conn.player.EXP,
				Rank:      conn.player.Rank,
				Health:    conn.player.Health,
				MaxHealth: conn.player.MaxHealth,
				UnitID:    conn.player.UnitID,
			}
		}
		conn.mu.RUnlock()
		if player != nil {
			break
		}
	}

	if player == nil {
		return // Player not found
	}

	// Calculate distance
	dx := player.CoordX - playerCoords[0]
	dy := player.CoordY - playerCoords[1]
	distance := math.Sqrt(float64(dx*dx + dy*dy))

	// Check if player is within MaxViewDistance
	if distance > MaxViewDistance {
		return
	}

	// Get binary player data
	binaryPlayer := getBinaryPlayer(player)
	playerData, err := b.EncodePlayer(binaryPlayer)
	if err == nil {
		gc.SendMessage(b.Message{
			Type: types.PlayerDataMessage,
			Data: playerData,
		})
	}
}

func (gc *GameConnection) handleChunkRequest(data any) {
	gc.mu.RLock()
	if gc.player == nil {
		gc.mu.RUnlock()
		gc.SendMessage(b.Message{
			Type:  types.ChunkRequestMessage,
			Error: "error.login.required",
		})
		return
	}

	// Calculate player's current chunk
	playerChunkX, playerChunkY := gc.player.GetChunkCoord(ChunkSize)
	gc.mu.RUnlock()

	// Convert data to ChunkRequest
	chunk, err := b.DecodeChunkRequest(data.([]byte))
	if err != nil {
		gc.SendMessage(b.Message{
			Type:  types.ChunkRequestMessage,
			Error: "error.invalid.request",
		})
		return
	}

	// Check if requested chunk is within MaxViewChunkDistance
	chunkDx := chunk.ChunkX - playerChunkX
	chunkDy := chunk.ChunkY - playerChunkY
	chunkDistance := math.Sqrt(float64(chunkDx*chunkDx + chunkDy*chunkDy))

	if chunkDistance > math.Hypot(float64(MaxViewChunkDistance), float64(MaxViewChunkDistance)) {
		return
	}

	// Calculate chunk boundaries
	startX := chunk.ChunkX * ChunkSize
	startY := chunk.ChunkY * ChunkSize
	endX := startX + ChunkSize
	endY := startY + ChunkSize

	chunkTiles := make([]b.ChunkTile, 0)

	gc.server.mu.RLock()
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
				})
			} else {
				chunkTiles = append(chunkTiles, b.ChunkTile{
					Type: uint8(types.TileTypeWater),
				}) // empty tile
			}
		}
	}
	gc.server.mu.RUnlock()

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
	gc.SendMessage(b.Message{
		Type: types.ChunkDataMessage,
		Data: chunkPacket,
	})
}

// handleDisconnect must be called with server mutex locked
func (gc *GameConnection) handleDisconnect() {
	gc.mu.RLock()
	log.Printf("Disconnected: %s\n", gc.udpAddr.String())

	// If player is not nil, save it
	var playerToSave *models.Player
	var playerCoords [2]uint16
	if gc.player != nil {
		playerToSave = gc.player
		playerCoords[0] = gc.player.CoordX
		playerCoords[1] = gc.player.CoordY
	}
	gc.mu.RUnlock()

	// Create connections copy before deleting (we already have server lock)
	connectionsCopy := make([]*GameConnection, 0, len(gc.server.connections))
	for conn := range gc.server.connections {
		connectionsCopy = append(connectionsCopy, conn)
	}

	// Delete connection (caller must have server mutex locked)
	delete(gc.server.connections, gc)

	if playerToSave != nil {
		if err := config.DB.Save(playerToSave).Error; err != nil {
			log.Printf("Error saving player %s: %v\n", playerToSave.Nickname, err)
		} else {
			log.Printf("Player %s saved successfully\n", playerToSave.Nickname)
		}

		// Send player left message to nearby players using internal broadcast
		data := make([]byte, 4)
		binary.LittleEndian.PutUint32(data, uint32(playerToSave.ID))
		gc.server.broadcastInRangeInternal(b.Message{
			Type: types.PlayerLeftMessage,
			Data: data,
		}, playerCoords[0], playerCoords[1], connectionsCopy)
	}
}

func (gc *GameConnection) handlePingPong(msg b.Message) {
	gc.SendMessage(msg)
}

func (gc *GameConnection) SendMessage(msg b.Message) error {
	if gc == nil || gc.udpConn == nil {
		return fmt.Errorf("invalid connection")
	}

	gc.mu.RLock()
	conn := gc.udpConn
	addr := gc.udpAddr
	gc.mu.RUnlock()

	rawData, err := b.EncodeRawMessage(msg)
	if err != nil {
		return err
	}

	messageID := nextMessageID()

	// Her parçaya 5 byte başlık eklenecek: [packetType:1][messageID:2][index:1][count:1]
	maxChunkSize := MaxUDPPayload - 5
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
		binary.Write(packet, binary.LittleEndian, messageID)
		packet.WriteByte(i)
		packet.WriteByte(totalChunks)

		packet.Write(chunk)

		if _, err := conn.WriteToUDP(packet.Bytes(), addr); err != nil {
			log.Printf("Error sending UDP packet (chunk %d/%d): %v", i+1, totalChunks, err)
			return err
		}
	}

	sentMessagesLock.Lock()
	defer sentMessagesLock.Unlock()
	ackRequired := slices.Contains(b.AckRequiredMessageTypes, msg.Type)
	sentMessage := &b.SentMessage{
		Chunks:         chunks,
		SentAt:         time.Now(),
		GameConnection: gc,
	}
	if ackRequired {
		sentMessage.Ack = make(chan bool, 1)
		go func(msgID uint32, msg *b.SentMessage) {
			retries := 0
			for retries < 3 {
				select {
				case <-msg.Ack:
					sentMessagesLock.Lock()
					delete(sentMessages, msgID)
					sentMessagesLock.Unlock()
					return
				case <-time.After(50 * time.Millisecond):
					fmt.Printf("ACK alınmadı. %d. kez yeniden gönderiliyor.\n", retries+1)
					resendMessage(msgID, msg)
					retries++
				}
			}

			fmt.Printf("Mesaj %d için ACK alınamadı, iptal ediliyor.\n", msgID)
			sentMessagesLock.Lock()
			delete(sentMessages, msgID)
			sentMessagesLock.Unlock()
		}(messageID, sentMessage)
	}
	sentMessages[messageID] = sentMessage

	return nil
}
func resendMessage(messageID uint32, msg *b.SentMessage) {
	for i, chunk := range msg.Chunks {
		packet := new(bytes.Buffer)
		packet.WriteByte(b.NormalPacket)
		binary.Write(packet, binary.LittleEndian, messageID)
		packet.WriteByte(byte(i))
		packet.WriteByte(byte(len(msg.Chunks)))
		packet.Write(chunk)

		conn := msg.GameConnection.(*GameConnection)
		conn.udpConn.WriteToUDP(packet.Bytes(), conn.udpAddr)
	}

	msg.SentAt = time.Now()
}

func (s *GameServer) autoSaveRoutine() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		s.mu.RLock()
		connectionsCopy := make([]*GameConnection, 0, len(s.connections))
		for conn := range s.connections {
			connectionsCopy = append(connectionsCopy, conn)
		}

		// Collect updated tiles
		updatedTiles := make([]models.MapTile, 0, len(s.updatedTiles))
		for _, tile := range s.updatedTiles {
			updatedTiles = append(updatedTiles, tile)
		}
		s.mu.RUnlock()

		// Clear updated tiles map after collecting (need write lock for this)
		s.mu.Lock()
		s.updatedTiles = make(map[string]models.MapTile)
		s.mu.Unlock()

		// Collect all active players
		activePlayers := make([]*models.Player, 0)
		for _, conn := range connectionsCopy {
			conn.mu.RLock()
			if conn.player != nil {
				activePlayers = append(activePlayers, conn.player)
			}
			conn.mu.RUnlock()
		}

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
	// First get player info safely
	gc.mu.RLock()
	if gc.player == nil {
		gc.mu.RUnlock()
		return
	}
	playerCoords := [2]uint16{gc.player.CoordX, gc.player.CoordY}
	playerID := gc.player.ID
	gc.mu.RUnlock()

	// Then get server data safely - separate the operations
	gc.server.mu.RLock()
	connectionsCopy := make([]*GameConnection, 0, len(gc.server.connections))
	for conn := range gc.server.connections {
		connectionsCopy = append(connectionsCopy, conn)
	}
	gc.server.mu.RUnlock()

	// Get countries separately to minimize lock time
	gc.server.mu.RLock()
	countriesCopy := make(map[uint8]models.Country)
	for id, country := range gc.server.countries {
		countriesCopy[id] = country
	}
	gc.server.mu.RUnlock()

	// Collect nearby players without holding server lock
	nearbyPlayers := make([]*b.Player, 0)
	for _, conn := range connectionsCopy {
		if conn == nil {
			continue
		}

		conn.mu.RLock()
		if conn.player == nil || conn.player.ID == playerID {
			conn.mu.RUnlock()
			continue
		}

		// Copy player data to avoid holding lock
		playerData := &models.Player{
			Model:     models.Model{ID: conn.player.ID},
			CoordX:    conn.player.CoordX,
			CoordY:    conn.player.CoordY,
			Nickname:  conn.player.Nickname,
			CountryID: conn.player.CountryID,
			EXP:       conn.player.EXP,
			Rank:      conn.player.Rank,
			Health:    conn.player.Health,
			MaxHealth: conn.player.MaxHealth,
			UnitID:    conn.player.UnitID,
		}
		conn.mu.RUnlock()

		dx := float64(playerData.CoordX - playerCoords[0])
		dy := float64(playerData.CoordY - playerCoords[1])
		distance := math.Sqrt(dx*dx + dy*dy)

		if distance <= MaxViewDistance {
			nearbyPlayers = append(nearbyPlayers, getBinaryPlayer(playerData))
		}
	}

	// Binary countries
	binaryCountries := make([]b.Country, 0)
	for _, country := range countriesCopy {
		binaryCountries = append(binaryCountries, getBinaryCountry(country))
	}

	data, err := b.EncodeSyncStateData(&b.SyncStateData{
		Players:     nearbyPlayers,
		Countries:   binaryCountries,
		OnlineCount: len(connectionsCopy),
	})
	if err != nil {
		return
	}

	// Send sync state message
	gc.SendMessage(b.Message{
		Type: types.SyncStateMessage,
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
	for _, country := range countries {
		server.countries[country.ID] = country
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
	// go cleanupOldSentMessages()

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
	if len(data) < 1 {
		return
	}

	var msg *b.Message
	switch data[0] {
	case b.NormalPacket:
		m, err := b.HandleIncomingPacket(data, conn, addr)
		if err != nil {
			log.Printf("Handle udp message error from %s, Error: %s\n", addr.String(), err)
			return
		}
		msg = m
	case b.ResendPacket:
		sentMessagesLock.RLock()
		defer sentMessagesLock.RUnlock()
		b.HandleResendRequest(data, sentMessages, conn, addr)
		return
	case b.AckPacket:
		sentMessagesLock.Lock()
		defer sentMessagesLock.Unlock()
		b.HandleAckRequest(data, sentMessages)
		return
	default:
		return
	}

	// Find or create game connection for this address
	gc := findOrCreateConnection(server, conn, addr)
	gc.mu.Lock()
	// Update last heartbeat time
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
		defer server.mu.Unlock()
		gc.handleDisconnect()
	case types.PingPongMessage:
		gc.handlePingPong(*msg)
	default:
		// unknown message
	}
}

func findOrCreateConnection(server *GameServer, conn *net.UDPConn, addr *net.UDPAddr) *GameConnection {
	server.mu.RLock()
	// Look for existing connection with this address
	for gc := range server.connections {
		gc.mu.RLock()
		if gc.udpAddr.String() == addr.String() {
			gc.mu.RUnlock()
			server.mu.RUnlock()
			return gc
		}
		gc.mu.RUnlock()
	}
	server.mu.RUnlock()

	// Create new connection if not found
	server.mu.Lock()
	defer server.mu.Unlock()

	// Double-check after acquiring write lock
	for gc := range server.connections {
		gc.mu.RLock()
		if gc.udpAddr.String() == addr.String() {
			gc.mu.RUnlock()
			return gc
		}
		gc.mu.RUnlock()
	}

	gc := NewGameConnection(addr, conn, server)
	fmt.Printf("New connection from %s\n", addr.String())

	// Make sure we don't exceed max connections
	if len(server.connections) >= MaxConnections {
		gc.SendMessage(b.Message{
			Type:  types.SystemMessage,
			Error: "error.server.full",
		})
		gc.handleDisconnect()
		return nil
	}

	server.connections[gc] = true
	return gc
}

func (s *GameServer) cleanupInactiveConnections() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		s.mu.RLock()
		connectionsCopy := make([]*GameConnection, 0, len(s.connections))
		for gc := range s.connections {
			connectionsCopy = append(connectionsCopy, gc)
		}
		s.mu.RUnlock()

		now := time.Now()
		connectionsToRemove := make([]*GameConnection, 0)

		for _, gc := range connectionsCopy {
			gc.mu.RLock()
			shouldRemove := now.Sub(gc.lastHeartbeat) > 30*time.Second
			gc.mu.RUnlock()

			if shouldRemove {
				connectionsToRemove = append(connectionsToRemove, gc)
			}
		}

		if len(connectionsToRemove) > 0 {
			s.mu.Lock()
			for _, gc := range connectionsToRemove {
				gc.handleDisconnect()
			}
			s.mu.Unlock()
		}
	}
}

func checkAndRequestMissingPackets() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		b.CheckAndRequestMissingPackets()
	}
}
