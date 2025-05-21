package socket

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"os"
	"projectt/config"
	"projectt/models"
	"projectt/models/request"
	"projectt/types"
	"strings"
	"sync"
	"time"
)

const (
	MaxConnections  = 100
	MaxViewDistance = 20
)

type GameConnection struct {
	conn   net.Conn
	player *models.Player
	server *GameServer
}

type GameServer struct {
	connections  map[*GameConnection]bool
	tiles        map[string]models.MapTile
	updatedTiles map[string]models.MapTile // Track tiles that need saving
	mu           sync.RWMutex
}

func NewGameServer() *GameServer {
	return &GameServer{
		connections:  make(map[*GameConnection]bool),
		tiles:        make(map[string]models.MapTile),
		updatedTiles: make(map[string]models.MapTile),
	}
}

func NewGameConnection(conn net.Conn, server *GameServer) *GameConnection {
	return &GameConnection{
		conn:   conn,
		server: server,
	}
}

func (s *GameServer) addConnection(gc *GameConnection) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.connections) >= MaxConnections {
		gc.SendMessage(Message{
			Type:  SystemMessage,
			Error: "error.server.full",
		})
		gc.conn.Close()
		return false
	}
	s.connections[gc] = true
	return true
}

func (s *GameServer) removeConnection(gc *GameConnection) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if gc.player != nil {
		if err := config.DB.Save(gc.player).Error; err != nil {
			log.Printf("Error saving player %s: %v\n", gc.player.Nickname, err)
		} else {
			log.Printf("Player %s saved successfully\n", gc.player.Nickname)
		}
	}
	delete(s.connections, gc)
}

// Broadcast sends a message to all connected clients
func (s *GameServer) Broadcast(msg Message) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for conn := range s.connections {
		// Skip disconnected clients
		if conn.conn == nil {
			continue
		}

		// Send message asynchronously to prevent blocking
		go func(c *GameConnection) {
			if err := c.SendMessage(msg); err != nil {
				fmt.Printf("Error broadcasting to %s: %v\n",
					c.conn.RemoteAddr().String(), err)
			}
		}(conn)
	}
}

// BroadcastInRange sends a message to all clients within MaxViewDistance
func (s *GameServer) BroadcastInRange(msg Message, centerX, centerY int) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for conn := range s.connections {
		// Skip disconnected clients or clients without players
		if conn.conn == nil || conn.player == nil {
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
					fmt.Printf("Error broadcasting to %s: %v\n",
						c.conn.RemoteAddr().String(), err)
				}
			}(conn)
		}
	}
}

func (gc *GameConnection) HandleConnection() {
	defer gc.conn.Close()
	defer gc.server.removeConnection(gc)

	// Add connection to server's connection pool
	if ok := gc.server.addConnection(gc); !ok {
		return
	}

	remoteAddr := gc.conn.RemoteAddr().String()
	fmt.Printf("New connection from %s\n", remoteAddr)

	// Send welcome message
	gc.SendMessage(Message{
		Type: UnauthorizedMessage,
	})

	// Main message loop
	for {
		msg, err := gc.ReadMessage()
		if err != nil {
			if gc.player != nil {
				fmt.Printf("Player %s disconnected (%s)\n", gc.player.Nickname, remoteAddr)
			} else {
				fmt.Printf("Client disconnected: %s\n", remoteAddr)
			}
			return
		}

		// Handle message based on type
		switch msg.Type {
		case LoginMessage:
			gc.handleLogin(msg.Data)
		case ChatMessage:
			gc.handleChat(msg.Data)
		case PlayerMovementMessage:
			gc.handleMovement(msg.Data)
		default:
			gc.SendMessage(Message{
				Type: UnknownMessage,
				Data: msg,
			})
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
			Data: map[string]any{
				"error": err.Error(),
			},
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

	// Send success message
	gc.SendMessage(Message{
		Type: LoginMessage,
		Data: map[string]any{
			"message": "success.login",
			"player":  gc.player,
		},
	})

	// Send initial sync state
	gc.sendSyncState()
}

func (gc *GameConnection) handleChat(data any) {
	if gc.player == nil {
		gc.SendMessage(Message{
			Type:  ChatMessage,
			Error: "error.login.required",
		})
		return
	}

	message, ok := data.(map[string]any)["message"].(string)
	if !ok {
		return
	}

	// Broadcast chat message to all clients
	gc.server.Broadcast(Message{
		Type: ChatMessage,
		Data: map[string]any{
			"from":    gc.player.Nickname,
			"message": message,
		},
	})
}

func (gc *GameConnection) handleMovement(data any) {
	if gc.player == nil {
		gc.SendMessage(Message{
			Type:  PlayerMovementMessage,
			Error: "error.login.required",
		})
		return
	}

	// Convert data to MovementRequest
	movementData, err := json.Marshal(data)
	if err != nil {
		gc.SendMessage(Message{
			Type:  PlayerMovementMessage,
			Error: "error.invalid.request",
		})
		return
	}

	var moveReq request.MovementRequest
	if err := json.Unmarshal(movementData, &moveReq); err != nil {
		gc.SendMessage(Message{
			Type:  PlayerMovementMessage,
			Error: "error.invalid.request",
		})
		return
	}

	// Calculate distance
	dx := moveReq.TargetX - gc.player.CoordX
	dy := moveReq.TargetY - gc.player.CoordY
	distance := math.Sqrt(float64(dx*dx + dy*dy))

	// Check if movement is within allowed range (e.g., 1 tile)
	if distance > 1.5 {
		gc.SendMessage(Message{
			Type:  PlayerMovementMessage,
			Error: "error.movement.too_far",
		})
		return
	}

	// Find target tile
	targetTileKey := fmt.Sprintf("%d,%d", moveReq.TargetX, moveReq.TargetY)

	gc.server.mu.RLock()
	targetTile, found := gc.server.tiles[targetTileKey]
	gc.server.mu.RUnlock()

	if !found || targetTile.TileType != types.TileTypeGround {
		gc.SendMessage(Message{
			Type:  PlayerMovementMessage,
			Error: "error.movement.invalid_tile",
		})
		return
	}

	// Update player position
	gc.player.CoordX = moveReq.TargetX
	gc.player.CoordY = moveReq.TargetY

	// Notify nearby clients about the movement
	gc.server.BroadcastInRange(Message{
		Type: PlayerMovementMessage,
		Data: map[string]any{
			"player":  gc.player,
			"coord_x": gc.player.CoordX,
			"coord_y": gc.player.CoordY,
		},
	}, gc.player.CoordX, gc.player.CoordY)

	// Send sync state to the player - EXPERIMENTAL
	gc.sendSyncState()
}

func (gc *GameConnection) ReadMessage() (Message, error) {
	var msg Message
	decoder := json.NewDecoder(gc.conn)
	err := decoder.Decode(&msg)

	if err != nil {
		// Check if connection is closed or broken
		if err == io.EOF || errors.Is(err, net.ErrClosed) ||
			strings.Contains(err.Error(), "connection reset by peer") ||
			strings.Contains(err.Error(), "broken pipe") {
			return Message{}, err
		}

		// Handle JSON decode errors
		return Message{
			Type:  UnknownMessage,
			Error: "error.invalid.json",
			Data: map[string]any{
				"error": err.Error(),
			},
		}, nil
	}

	return msg, nil
}

func (gc *GameConnection) SendMessage(msg Message) error {
	return json.NewEncoder(gc.conn).Encode(msg)
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

	// Collect nearby tiles more efficiently
	nearbyTiles := make([]models.MapTile, 0)

	// Calculate bounds for tile checking
	minX := gc.player.CoordX - int(MaxViewDistance)
	maxX := gc.player.CoordX + int(MaxViewDistance)
	minY := gc.player.CoordY - int(MaxViewDistance)
	maxY := gc.player.CoordY + int(MaxViewDistance)

	// Only check tiles within the square bounds
	for x := minX; x <= maxX; x++ {
		for y := minY; y <= maxY; y++ {
			// Calculate actual distance
			dx := float64(x - gc.player.CoordX)
			dy := float64(y - gc.player.CoordY)
			distance := math.Sqrt(dx*dx + dy*dy)

			if distance <= MaxViewDistance {
				// Try to get tile at this coordinate
				if tile, exists := gc.server.tiles[fmt.Sprintf("%d,%d", x, y)]; exists {
					nearbyTiles = append(nearbyTiles, tile)
				}
			}
		}
	}

	// Collect nearby players
	nearbyPlayers := make([]*models.Player, 0)
	for conn := range gc.server.connections {
		if conn.player == nil || conn.player.ID == gc.player.ID {
			continue
		}

		dx := float64(conn.player.CoordX - gc.player.CoordX)
		dy := float64(conn.player.CoordY - gc.player.CoordY)
		distance := math.Sqrt(dx*dx + dy*dy)

		if distance <= MaxViewDistance {
			nearbyPlayers = append(nearbyPlayers, conn.player)
		}
	}

	// Send player movement message to nearby players for let them know
	gc.server.BroadcastInRange(Message{
		Type: PlayerMovementMessage,
		Data: map[string]any{
			"player":  gc.player,
			"coord_x": gc.player.CoordX,
			"coord_y": gc.player.CoordY,
		},
	}, gc.player.CoordX, gc.player.CoordY)

	// Send sync state message
	gc.SendMessage(Message{
		Type: SyncStateMessage,
		Data: map[string]any{
			"tiles":             nearbyTiles,
			"players":           nearbyPlayers,
			"connected_clients": len(gc.server.connections),
			"view_distance":     MaxViewDistance,
		},
	})
}

func StartServer() {
	port := os.Getenv("APP_PORT")
	server := NewGameServer()

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

	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("Server could not be started: %v", err)
	}
	defer listener.Close()

	fmt.Printf("Socket server is running on port %s...\n", port)

	// Start auto-save routine
	go server.autoSaveRoutine()

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Connection could not be accepted: %v", err)
			continue
		}

		gc := NewGameConnection(conn, server)
		go gc.HandleConnection()
	}
}
