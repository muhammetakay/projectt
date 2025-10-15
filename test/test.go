package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"projectt/test/message"
	"strconv"
	"syscall"
	"time"
)

func main() {
	// Önce TCP bağlantısı oluştur
	tcpConn, err := net.Dial("tcp", "127.0.0.1:8080")
	if err != nil {
		panic(err)
	}
	defer tcpConn.Close()

	// Welcome mesajını oku
	welcomeResponse := make([]byte, 1024)
	n, err := tcpConn.Read(welcomeResponse)
	if err != nil {
		panic(err)
	}

	// Welcome mesajını decode et
	welcomeMsg, err := message.DecodeRawMessage(welcomeResponse[4:n])
	if err != nil {
		panic(err)
	}

	// Welcome mesajından connection ID'yi al
	var connID uint32
	buf := bytes.NewReader(welcomeMsg.Data)

	// ConnectionID (4 byte)
	if err := binary.Read(buf, binary.LittleEndian, &connID); err != nil {
		log.Fatalf("Can't read connection id from data")
	}
	fmt.Printf("Received connection ID: %d\n", connID)

	// UDP bağlantısı oluştur
	conn, err := net.DialUDP("udp", nil, &net.UDPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: 8080,
	})
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	go func() {
		// Read response
		response := make([]byte, 65535) // UDP için maksimum paket boyutu
		n, _, err := conn.ReadFromUDP(response)
		if err != nil {
			panic(err)
		}

		// Decode and handle message
		msg, err := message.DecodeRawMessage(response[:n])
		if err != nil {
			panic(err)
		}

		// Parse received timestamp as nanoseconds
		receivedNano, err := strconv.ParseInt(string(msg.Data), 10, 64)
		if err != nil {
			panic(err)
		}

		// Calculate difference in milliseconds with decimal points
		unixNanotime := time.Now().UnixNano()
		fmt.Printf("Current Unix Nano Time: %d\n", unixNanotime)
		fmt.Printf("Received Nano Time: %d\n", receivedNano)
		diffNano := unixNanotime - receivedNano
		diffMs := float64(diffNano) / float64(time.Millisecond)

		fmt.Printf("Received response: Type=%d, Data=%s, Error=%s\n", msg.Type, string(msg.Data), msg.Error)
		fmt.Printf("Time difference: %.3f ms\n", diffMs)
	}()

	// Get current timestamp in nanoseconds
	timestamp := time.Now().UnixNano()
	msgReq := message.Message{
		Type: message.PingPongMessage,
		Data: []byte(fmt.Sprintf("%d", timestamp)),
	}

	rawData, err := message.EncodeRawMessage(msgReq)
	if err != nil {
		panic(err)
	}

	// Combine connID and data into a single buffer
	messageBuffer := make([]byte, 4+len(rawData))
	binary.LittleEndian.PutUint32(messageBuffer[:4], uint32(connID))
	copy(messageBuffer[4:], rawData)

	// Send message
	_, err = conn.Write(messageBuffer)
	if err != nil {
		panic(err)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	fmt.Println("Shutting down server...")
}
