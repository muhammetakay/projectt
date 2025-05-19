package socket

import (
	"fmt"
	"log"
	"net"
	"os"
)

func handleConnection(conn net.Conn) {
	defer conn.Close()

	// Print client information
	fmt.Printf("New connection: %s\n", conn.RemoteAddr().String())

	// Buffer to read data from connection
	buffer := make([]byte, 1024)

	for {
		// Read data
		n, err := conn.Read(buffer)
		if err != nil {
			fmt.Printf("Client connection closed: %s\n", conn.RemoteAddr().String())
			return
		}

		// Print received message
		message := string(buffer[:n])
		fmt.Printf("Message from client: %s\n", message)

		// Send response to client
		response := "Message received: " + message
		conn.Write([]byte(response))
	}
}

func StartServer() {
	port := os.Getenv("APP_PORT")

	// Start socket server
	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("Server could not be started: %v", err)
	}
	defer listener.Close()

	fmt.Println("Socket server is running on port 8080...")

	// Continuously accept new connections
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Connection could not be accepted: %v", err)
			continue
		}

		// Start a new goroutine for each connection
		go handleConnection(conn)
	}
}
