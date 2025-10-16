package socket

import (
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"math/rand/v2"
	"net"
	"os"
	b "projectt/binary"
	"projectt/config"
	"projectt/models"
	"projectt/types"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	server *GameServer
)

type GameConnection struct {
	// TCP client connection
	conn net.Conn
	// UDP client connection
	udpAddr *net.UDPAddr
	udpConn *net.UDPConn
	// Unique connection ID
	connID uint32

	player *models.Player
	server *GameServer
	mu     sync.RWMutex // Mutex for thread-safe access

	lastHeartbeat time.Time
}

type GameServer struct {
	connections   map[uint32]*GameConnection
	countries     map[uint8]models.Country
	tiles         map[string]models.MapTile
	updatedTiles  map[string]models.MapTile // Track tiles that need saving
	movingPlayers map[uint]*models.Player
	mu            sync.RWMutex
}

func NewGameServer() *GameServer {
	return &GameServer{
		connections:   make(map[uint32]*GameConnection),
		countries:     make(map[uint8]models.Country),
		tiles:         make(map[string]models.MapTile),
		updatedTiles:  make(map[string]models.MapTile),
		movingPlayers: make(map[uint]*models.Player),
	}
}

func NewGameConnection(conn net.Conn, server *GameServer) *GameConnection {
	// Generate a unique connection ID
	var connID uint32
	for {
		connID = rand.Uint32()
		unique := true
		server.mu.RLock()
		_, exists := server.connections[connID]
		if exists {
			unique = false
		}
		server.mu.RUnlock()

		if unique {
			break
		}
	}
	return &GameConnection{
		conn:    conn,
		udpAddr: nil,
		udpConn: nil,
		connID:  connID,
		server:  server,
	}
}

// Broadcast sends a message to all connected clients
func (s *GameServer) Broadcast(msg b.Message) {
	s.mu.RLock()
	connectionsCopy := make([]*GameConnection, 0, len(s.connections))
	for _, conn := range s.connections {
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
			if err := c.SendTCPMessage(msg); err != nil {
				log.Printf("Error broadcasting to %s: %v\n",
					c.conn.RemoteAddr().String(), err)
			}
		}(conn)
	}
}

// BroadcastInRange sends a message to all clients within MaxViewDistance
func (s *GameServer) BroadcastInRange(msg b.Message, centerX, centerY float32, useTCP bool) {
	s.mu.RLock()
	connectionsCopy := make([]*GameConnection, 0, len(s.connections))
	for _, conn := range s.connections {
		connectionsCopy = append(connectionsCopy, conn)
	}
	s.mu.RUnlock()

	s.broadcastInRangeInternal(msg, centerX, centerY, connectionsCopy, useTCP)
}

// broadcastInRangeInternal performs the actual broadcast without acquiring server locks
// This is useful when the caller already holds the server lock
func (s *GameServer) broadcastInRangeInternal(msg b.Message, centerX, centerY float32, connections []*GameConnection, useTCP bool) {
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
			dx := c.player.CoordX - centerX
			dy := c.player.CoordY - centerY
			distance := math.Sqrt(float64(dx*dx) + float64(dy*dy))
			playerWithinRange := distance <= float64(config.MaxViewDistance)
			c.mu.RUnlock()

			// Send only if within view distance
			if playerWithinRange {
				var err error
				if useTCP {
					err = c.SendTCPMessage(msg)
				} else {
					err = c.SendUDPMessage(msg)
				}
				if err != nil {
					log.Printf("Error broadcasting to %s: %v\n",
						c.conn.RemoteAddr().String(), err)
				}
			} else {
				// log.Printf("Player %s (%f, %f) not in range (%f, %f) - distance: %f - max distance: %d", c.player.Nickname, c.player.CoordX, c.player.CoordY, centerX, centerY, distance, config.MaxViewDistance)
			}
		}(conn)
	}
}

func (gc *GameConnection) handleLogin(data []byte) {
	loginRequest, err := b.DecodeLoginMessage(data)
	if err != nil {
		gc.SendTCPMessage(b.Message{
			Type:  types.LoginMessage,
			Error: "error.request.invalid",
		})
		return
	}

	// Validate request
	if err := loginRequest.Validate(); err != nil {
		gc.SendTCPMessage(b.Message{
			Type:  types.LoginMessage,
			Error: err.Error(),
		})
		return
	}

	// check if player with nickname already connected
	gc.server.mu.RLock()
	connectedNicknames := make(map[string]bool)
	for _, conn := range gc.server.connections {
		conn.mu.RLock()
		if conn.player != nil {
			connectedNicknames[strings.ToLower(conn.player.Nickname)] = true
		}
		conn.mu.RUnlock()
	}
	gc.server.mu.RUnlock()

	if connectedNicknames[strings.ToLower(loginRequest.Nickname)] {
		gc.SendTCPMessage(b.Message{
			Type:  types.LoginMessage,
			Error: "error.player.already_connected",
		})
		return
	}

	gc.mu.Lock()
	if err := config.DB.Where("nickname ILIKE ?", loginRequest.Nickname).First(&gc.player).Error; err != nil {
		gc.player = nil // reset player if not found
		gc.mu.Unlock()
		gc.SendTCPMessage(b.Message{
			Type:  types.LoginMessage,
			Error: "error.player.not_found",
		})
		return
	}
	gc.mu.Unlock()

	binaryPlayer := getBinaryPlayer(gc.player)
	player, err := b.EncodePlayer(binaryPlayer)
	if err != nil {
		gc.SendTCPMessage(b.Message{
			Type:  types.LoginMessage,
			Error: "error.login.unavailable",
		})
		return
	}

	// Send success message
	gc.SendTCPMessage(b.Message{
		Type: types.LoginMessage,
		Data: player,
	})

	// send player joined message to nearby players
	gc.server.BroadcastInRange(b.Message{
		Type: types.PlayerJoinedMessage,
		Data: player,
	}, gc.player.CoordX, gc.player.CoordY, true)

	// send initial data
	gc.sendSyncState()
}

func (gc *GameConnection) handleChat(data []byte) {
	gc.mu.RLock()
	defer gc.mu.RUnlock()
	if gc.player == nil {
		gc.SendTCPMessage(b.Message{
			Type:  types.ChatMessage,
			Error: "error.login.required",
		})
		return
	}

	msg, err := b.DecodeChatMessage(data)
	if err != nil {
		gc.SendTCPMessage(b.Message{
			Type:  types.ChatMessage,
			Error: "error.chat.invalid",
		})
		return
	}

	if len(msg.Message) == 0 {
		gc.SendTCPMessage(b.Message{
			Type:  types.ChatMessage,
			Error: "error.chat.empty",
		})
		return
	}

	if msg.Message[0] == '/' {
		// Handle commands here (e.g., /help, /whisper, etc.)
		parts := strings.Fields(msg.Message)
		switch parts[0] {
		case "/help":
			gc.SendTCPMessage(b.Message{
				Type:  types.ChatMessage,
				Error: "Help is on the way!",
			})

		case "/w":
			if len(parts) < 3 {
				gc.SendTCPMessage(b.Message{
					Type:  types.ChatMessage,
					Error: "error.chat.usage_whisper",
				})
				return
			}

			targetPlayer := parts[1]
			whisperMessage := strings.Join(parts[2:], " ")
			gc.SendTCPMessage(b.Message{
				Type:  types.ChatMessage,
				Error: fmt.Sprintf("Whispering to %s: %s", targetPlayer, whisperMessage),
			})

		case "/notice":
			if len(parts) == 1 {
				gc.SendTCPMessage(b.Message{
					Type:  types.ChatMessage,
					Error: "error.chat.usage_notice",
				})
				return
			}
			message := strings.Join(parts[1:], " ")
			chatMessage := b.ChatMessage{
				Type:    b.ChatMessageTypeNotice,
				From:    "Notice",
				Message: message,
			}
			data, err := b.EncodeChatMessage(&chatMessage)
			if err == nil {
				// Broadcast chat message to all clients
				gc.server.Broadcast(b.Message{
					Type: types.ChatMessage,
					Data: data,
				})
			}

		case "/tp":
			if len(parts) != 4 {
				gc.SendTCPMessage(b.Message{
					Type:  types.ChatMessage,
					Error: "error.chat.usage_tp",
				})
				return
			}
			targetPlayer := strings.ToLower(parts[1])
			x, y := parts[2], parts[3]

			var p *models.Player
			if targetPlayer == "@me" {
				p = gc.player
			} else {
				gc.server.mu.RLock()
				for _, conn := range gc.server.connections {
					if conn == nil || conn.player == nil {
						continue
					}
					if strings.ToLower(conn.player.Nickname) == targetPlayer {
						p = conn.player
						break
					}
				}
				gc.server.mu.RUnlock()
			}

			if p == nil {
				gc.SendTCPMessage(b.Message{
					Type:  types.ChatMessage,
					Error: "error.general.player_not_found",
				})
				return
			}

			var cx, cy int
			if cx, err = strconv.Atoi(x); err != nil {
				gc.SendTCPMessage(b.Message{
					Type:  types.ChatMessage,
					Error: "error.chat.usage_tp",
				})
				return
			}
			if cy, err = strconv.Atoi(y); err != nil {
				gc.SendTCPMessage(b.Message{
					Type:  types.ChatMessage,
					Error: "error.chat.usage_tp",
				})
				return
			}
			p.CoordX, p.CoordY = float32(cx), float32(cy)
			p.LastUpdated = time.Now()
			// update moving players
			gc.server.mu.Lock()
			gc.server.movingPlayers[p.ID] = p
			gc.server.mu.Unlock()
			// send chat message to player
			chatMessage := b.ChatMessage{
				Type:    b.ChatMessageTypeSystem,
				From:    "System",
				Message: fmt.Sprintf("Player %s successfully teleported to %s,%s", p.Nickname, x, y),
			}
			data, err := b.EncodeChatMessage(&chatMessage)
			if err == nil {
				gc.SendTCPMessage(b.Message{
					Type: types.ChatMessage,
					Data: data,
				})
			}

		default:
			gc.SendTCPMessage(b.Message{
				Type:  types.ChatMessage,
				Error: "error.chat.unknown_command",
			})
		}
	} else {
		chatMessage := b.ChatMessage{
			Type:    b.ChatMessageTypeGeneral,
			From:    gc.player.Nickname,
			Message: msg.Message,
		}
		data, err := b.EncodeChatMessage(&chatMessage)
		if err != nil {
			gc.SendTCPMessage(b.Message{
				Type:  types.ChatMessage,
				Error: "error.chat.encoding_failed",
			})
			return
		}
		// Broadcast chat message to all clients
		gc.server.Broadcast(b.Message{
			Type: types.ChatMessage,
			Data: data,
		})
	}
}

func (gc *GameConnection) handleMovement(data any) {
	gc.mu.Lock()
	defer gc.mu.Unlock()
	if gc.player == nil {
		gc.mu.Unlock()
		gc.SendTCPMessage(b.Message{
			Type:  types.UnauthorizedMessage,
			Error: "error.login.required",
		})
		return
	}

	// Convert data to PlayerMovementRequest
	moveReq, err := b.DecodePlayerMovementRequest(data.([]byte))
	if err != nil {
		return
	}

	// Check if request is old
	if gc.player.LastUpdatedTicks > moveReq.Timestamp {
		return // return, already have more current input
	}

	// Update player
	gc.player.DirX, gc.player.DirY = moveReq.DirX, moveReq.DirY
	gc.player.LastUpdatedTicks = moveReq.Timestamp
	gc.player.LastUpdated = time.Now()

	// Update moving players
	gc.server.mu.Lock()
	if gc.player.IsMoving() {
		gc.server.movingPlayers[gc.player.ID] = gc.player
	}
	gc.server.mu.Unlock()
}

func (gc *GameConnection) handlePlayerData(data any) {
	gc.mu.RLock()
	if gc.player == nil {
		gc.mu.RUnlock()
		gc.SendTCPMessage(b.Message{
			Type:  types.UnauthorizedMessage,
			Error: "error.login.required",
		})
		return
	}
	playerCoords := [2]float32{gc.player.CoordX, gc.player.CoordY}
	gc.mu.RUnlock()

	// Convert data to PlayerDataRequest
	req, err := b.DecodePlayerDataRequest(data.([]byte))
	if err != nil {
		return
	}

	gc.server.mu.RLock()
	connectionsCopy := make([]*GameConnection, 0, len(gc.server.connections))
	for _, conn := range gc.server.connections {
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
			player = conn.player.Copy()
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
	if distance > float64(config.MaxViewDistance) {
		return
	}

	// Get binary player data
	binaryPlayer := getBinaryPlayer(player)
	playerData, err := b.EncodePlayer(binaryPlayer)
	if err == nil {
		gc.SendTCPMessage(b.Message{
			Type: types.PlayerDataMessage,
			Data: playerData,
		})
	}
}

func (gc *GameConnection) handleChunkRequest(data any) {
	gc.mu.RLock()
	if gc.player == nil {
		gc.mu.RUnlock()
		gc.SendTCPMessage(b.Message{
			Type:  types.ChunkRequestMessage,
			Error: "error.login.required",
		})
		return
	}

	// Calculate player's current chunk
	playerChunkX, playerChunkY := gc.player.GetChunkCoord(config.ChunkSize)
	gc.mu.RUnlock()

	// Convert data to ChunkRequest
	chunk, err := b.DecodeChunkRequest(data.([]byte))
	if err != nil {
		gc.SendTCPMessage(b.Message{
			Type:  types.ChunkRequestMessage,
			Error: "error.invalid.request",
		})
		return
	}

	// Check if requested chunk is within MaxViewChunkDistance
	chunkDx := chunk.ChunkX - playerChunkX
	chunkDy := chunk.ChunkY - playerChunkY
	chunkDistance := math.Sqrt(float64(chunkDx*chunkDx + chunkDy*chunkDy))

	if chunkDistance > math.Hypot(float64(config.MaxChunkViewDistance), float64(config.MaxChunkViewDistance)) {
		return
	}

	// Calculate chunk boundaries
	startX := chunk.ChunkX * uint16(config.ChunkSize)
	startY := chunk.ChunkY * uint16(config.ChunkSize)
	endX := startX + uint16(config.ChunkSize)
	endY := startY + uint16(config.ChunkSize)

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

	// check chunkTiles array length if is it equals to 256
	if len(chunkTiles) != 256 {
		return
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
	gc.SendTCPMessage(b.Message{
		Type: types.ChunkDataMessage,
		Data: chunkPacket,
	})
}

// handleDisconnect must be called with server mutex locked
func (gc *GameConnection) handleDisconnect() {
	gc.mu.RLock()
	log.Printf("Disconnected: %s\n", gc.conn.RemoteAddr().String())

	// If player is not nil, save it
	var playerToSave *models.Player
	var playerCoords [2]float32
	if gc.player != nil {
		playerToSave = gc.player
		playerCoords[0] = gc.player.CoordX
		playerCoords[1] = gc.player.CoordY
	}
	gc.mu.RUnlock()

	// Create connections copy before deleting (we already have server lock)
	connectionsCopy := make([]*GameConnection, 0, len(gc.server.connections))
	for _, conn := range gc.server.connections {
		connectionsCopy = append(connectionsCopy, conn)
	}

	// Delete connection (caller must have server mutex locked)
	delete(gc.server.connections, gc.connID)

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
		}, playerCoords[0], playerCoords[1], connectionsCopy, true)
	}
}

func (gc *GameConnection) handlePingPong(msg b.Message) {
	gc.SendUDPMessage(msg)
}

func (gc *GameConnection) SendTCPMessage(msg b.Message) error {
	if gc == nil || gc.conn == nil {
		return fmt.Errorf("invalid connection")
	}

	gc.mu.RLock()
	conn := gc.conn
	gc.mu.RUnlock()

	rawData, err := b.EncodeRawMessage(msg)
	if err != nil {
		return err
	}

	// Combine length and data into a single buffer
	messageBuffer := make([]byte, 4+len(rawData))
	binary.LittleEndian.PutUint32(messageBuffer[:4], uint32(len(rawData)))
	copy(messageBuffer[4:], rawData)

	// Write entire message in a single call
	if _, err := conn.Write(messageBuffer); err != nil {
		return fmt.Errorf("failed to write message: %v", err)
	}

	return nil
}

func (gc *GameConnection) SendUDPMessage(msg b.Message) error {
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

	// Write entire message in a single call
	if _, err := conn.WriteToUDP(rawData, addr); err != nil {
		return fmt.Errorf("failed to write message: %v", err)
	}

	return nil
}

func (s *GameServer) autoSaveRoutine() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		Save()
	}
}

func Save() {
	server.mu.RLock()
	connectionsCopy := make([]*GameConnection, 0, len(server.connections))
	for _, conn := range server.connections {
		connectionsCopy = append(connectionsCopy, conn)
	}

	// Collect updated tiles
	updatedTiles := make([]models.MapTile, 0, len(server.updatedTiles))
	for _, tile := range server.updatedTiles {
		updatedTiles = append(updatedTiles, tile)
	}
	server.mu.RUnlock()

	// Clear updated tiles map after collecting (need write lock for this)
	server.mu.Lock()
	server.updatedTiles = make(map[string]models.MapTile)
	server.mu.Unlock()

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
		return
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
	playerCoords := [2]float32{gc.player.CoordX, gc.player.CoordY}
	playerID := gc.player.ID
	gc.mu.RUnlock()

	// Then get server data safely - separate the operations
	gc.server.mu.RLock()
	connectionsCopy := make([]*GameConnection, 0, len(gc.server.connections))
	for _, conn := range gc.server.connections {
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
		playerData := conn.player.Copy()
		conn.mu.RUnlock()

		dx := float64(playerData.CoordX - playerCoords[0])
		dy := float64(playerData.CoordY - playerCoords[1])
		distance := math.Sqrt(dx*dx + dy*dy)

		if distance <= float64(config.MaxViewDistance) {
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
	gc.SendTCPMessage(b.Message{
		Type: types.SyncStateMessage,
		Data: data,
	})
}

func StartServer() {
	port := os.Getenv("APP_PORT")
	server = NewGameServer()

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

	// Start auto-save routine
	go server.autoSaveRoutine()
	// Start cleanup routine
	go server.cleanupInactiveConnections()
	// Start tick loop
	go server.tickLoop()

	// TCP setup
	go func() {
		listener, err := net.Listen("tcp", ":"+port)
		if err != nil {
			log.Fatalf("Server could not be started: %v", err)
		}
		defer listener.Close()

		fmt.Printf("TCP server is running on port %s...\n", port)

		for {
			conn, err := listener.Accept()
			if err != nil {
				log.Printf("Error accepting connection: %v", err)
				continue
			}

			// Handle connection
			go handleTCPConnection(server, conn)
		}
	}()

	// UDP setup
	go func() {
		addr, err := net.ResolveUDPAddr("udp", ":"+port)
		if err != nil {
			log.Fatalf("Failed to resolve UDP address: %v", err)
		}

		udpConn, err := net.ListenUDP("udp", addr)
		if err != nil {
			log.Fatalf("Failed to start UDP server: %v", err)
		}
		defer udpConn.Close()

		fmt.Printf("UDP server is running on port %s...\n", port)

		buffer := make([]byte, 4096)
		for {
			n, remoteAddr, err := udpConn.ReadFromUDP(buffer)
			if err != nil {
				log.Printf("Error reading from UDP connection: %v", err)
				continue
			}

			go handleUDPConnection(server, udpConn, remoteAddr, buffer[:n])
		}
	}()
}

func (s *GameServer) cleanupInactiveConnections() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		s.mu.RLock()
		connectionsCopy := make([]*GameConnection, 0, len(s.connections))
		for _, gc := range s.connections {
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

func (s *GameServer) tickLoop() {
	duration := config.FixedDeltaTime
	ticker := time.NewTicker(duration)
	defer ticker.Stop()

	// Get tiles safely
	s.mu.RLock()
	tiles := s.tiles
	s.mu.RUnlock()

	for range ticker.C {
		// Get moving players safely with server lock
		s.mu.RLock()
		playersCopy := make([]*models.Player, 0)
		for _, player := range s.movingPlayers {
			if player == nil {
				continue
			}
			playersCopy = append(playersCopy, player)
		}
		s.mu.RUnlock()

		// Check player last updated time and delete if not moving more than a second
		players := make([]*models.Player, 0)
		for _, player := range playersCopy {
			if player == nil {
				continue
			}
			diff := time.Since(player.LastUpdated)
			if diff.Seconds() >= 1 {
				s.mu.Lock()
				delete(s.movingPlayers, player.ID)
				s.mu.Unlock()
				continue
			}
			players = append(players, player)
		}

		if len(players) == 0 {
			continue
		}

		// Calculate and send movement data
		for _, player := range players {
			if player == nil {
				continue
			}

			// Calculate time delta in seconds since last update
			func() {
				cx, cy := player.CoordX, player.CoordY

				// Get movement speed
				speed := player.GetCurrentSpeed()

				// Normalize direction vector
				magnitude := float32(math.Sqrt(float64(player.DirX*player.DirX + player.DirY*player.DirY)))
				if magnitude > 0 {
					normalizedDirX := player.DirX / magnitude
					normalizedDirY := player.DirY / magnitude

					// Calculate position based on normalized direction, speed and delta time
					cx += normalizedDirX * speed * float32(config.FixedDeltaTime.Seconds())
					cy += normalizedDirY * speed * float32(config.FixedDeltaTime.Seconds())
				}

				// Check if new position is walkable
				tileKey := fmt.Sprintf("%d,%d", uint16(cx), uint16(cy))

				tile, exists := tiles[tileKey]
				if !exists {
					// Not exists, skip
					return
				}
				// Check if tile is suitable for the unit type
				switch player.GetUnitType() {
				case types.UnitTypeInfantry, types.UnitTypeTank:
					if tile.TileType != types.TileTypeGround {
						// Cannot walk on non-ground tiles
						return
					}
				case types.UnitTypeShip, types.UnitTypeBattleShip:
					if tile.TileType != types.TileTypeWater {
						// Cannot sail on non-water tiles
						return
					}

				}

				// Update player
				player.CoordX, player.CoordY = cx, cy
			}()

			playerMovementData := b.PlayerMovementData{
				PlayerID:         uint32(player.ID),
				PosX:             player.CoordX,
				PosY:             player.CoordY,
				DirX:             player.DirX,
				DirY:             player.DirY,
				Speed:            player.GetCurrentSpeed(),
				LastUpdatedTicks: player.LastUpdatedTicks,
			}

			// Notify nearby clients about movement start
			encodedData := b.EncodePlayerMovementData(&playerMovementData)
			s.BroadcastInRange(b.Message{
				Type: types.PlayerMovementMessage,
				Data: encodedData,
			}, player.CoordX, player.CoordY, false)
		}
	}
}
