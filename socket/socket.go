package socket

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
)

type GameConnection struct {
	conn     net.Conn
	nickname string
	server   *GameServer
}

type GameServer struct {
	connections map[*GameConnection]bool
	mu          sync.RWMutex
}

func NewGameServer() *GameServer {
	return &GameServer{
		connections: make(map[*GameConnection]bool),
	}
}

func NewGameConnection(conn net.Conn, server *GameServer) *GameConnection {
	return &GameConnection{
		conn:   conn,
		server: server,
	}
}

func (s *GameServer) addConnection(gc *GameConnection) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connections[gc] = true
}

func (s *GameServer) removeConnection(gc *GameConnection) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.connections, gc)
}

func (gc *GameConnection) HandleConnection() {
	defer gc.conn.Close()
	defer gc.server.removeConnection(gc)

	gc.server.addConnection(gc)
	fmt.Printf("New connection: %s\n", gc.conn.RemoteAddr().String())

	// Send welcome message
	gc.SendMessage(Message{
		Type: UnauthorizedMessage,
		Data: nil,
	})

	// Main message loop
	for {
		msg, err := gc.ReadMessage()
		if err != nil {
			fmt.Printf("Client disconnected: %s\n", gc.conn.RemoteAddr().String())
			return
		}

		// Handle message based on type
		switch msg.Type {
		case LoginMessage:
			gc.handleLogin(msg.Data)
		case ChatMessage:
			gc.handleChat(msg.Data)
		default:
			gc.SendMessage(Message{
				Type: UnknownMessage,
				Data: nil,
			})
		}
	}
}

func (gc *GameConnection) handleLogin(data any) {
	username, ok := data.(map[string]any)["username"].(string)
	if !ok {
		gc.SendMessage(Message{
			Type: SystemMessage,
			Data: map[string]any{
				"message": "error.username.invalid",
			},
		})
		return
	}

	// TODO: Add password validation here

	gc.nickname = username
	gc.SendMessage(Message{
		Type: SystemMessage,
		Data: map[string]any{
			"message":  "success.login",
			"username": username,
		},
	})
}

func (gc *GameConnection) handleChat(data any) {
	if gc.nickname == "" {
		gc.SendMessage(Message{
			Type: SystemMessage,
			Data: map[string]any{
				"message": "error.login.required",
			},
		})
		return
	}

	message, ok := data.(map[string]any)["message"].(string)
	if !ok {
		return
	}

	fmt.Printf("Message from %s: %s\n", gc.nickname, message)
	gc.SendMessage(Message{
		Type: ChatMessage,
		Data: map[string]any{
			"from":    gc.nickname,
			"message": message,
		},
	})
}

func (gc *GameConnection) ReadMessage() (Message, error) {
	var msg Message
	decoder := json.NewDecoder(gc.conn)
	err := decoder.Decode(&msg)

	if err != nil {
		return Message{
			Type: UnknownMessage,
			Data: nil,
		}, nil
	}

	return msg, nil
}

func (gc *GameConnection) SendMessage(msg Message) error {
	return json.NewEncoder(gc.conn).Encode(msg)
}

func StartServer() {
	port := os.Getenv("APP_PORT")
	server := NewGameServer()

	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("Server could not be started: %v", err)
	}
	defer listener.Close()

	fmt.Printf("Socket server is running on port %s...\n", port)

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
