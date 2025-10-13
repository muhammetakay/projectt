package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"projectt/test/message"
	"strconv"
	"time"
)

func main() {
	conn, err := net.Dial("tcp", "localhost:8080")
	if err != nil {
		panic(err)
	}
	defer conn.Close()

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

	// Combine length and data into a single buffer
	messageBuffer := make([]byte, 4+len(rawData))
	binary.LittleEndian.PutUint32(messageBuffer[:4], uint32(len(rawData)))
	copy(messageBuffer[4:], rawData)

	// Send message
	_, err = conn.Write(messageBuffer)
	if err != nil {
		panic(err)
	}

	// Read response
	// Read message length (4 bytes)
	lenBuf := make([]byte, 4)
	_, err = io.ReadFull(conn, lenBuf)
	if err != nil {
		panic(err)
	}
	messageLen := binary.LittleEndian.Uint32(lenBuf)

	// Read message data
	data := make([]byte, messageLen)
	_, err = io.ReadFull(conn, data)
	if err != nil {
		panic(err)
	}

	// Decode and handle message
	msg, err := message.DecodeRawMessage(data)
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
}
