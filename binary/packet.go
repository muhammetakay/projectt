package binary

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

type ReceivedMessage struct {
	TotalChunks byte
	Chunks      [][]byte
	Received    byte
	LastUpdate  time.Time

	Conn *net.UDPConn
	Addr *net.UDPAddr

	MissingTries map[byte]int       // index → kaç kere istenmiş
	LastRequest  map[byte]time.Time // index → son istek zamanı
}

type SentMessage struct {
	Chunks [][]byte
	SentAt time.Time
}

var (
	packetBuffer     = make(map[uint16]*ReceivedMessage)
	packetBufferLock sync.RWMutex
)

var packetTimeout = 2 * time.Second

const (
	NormalPacket = 0x01
	ResendPacket = 0xFE
	AckPacket    = 0xFD
)

func HandleIncomingPacket(packet []byte, conn *net.UDPConn, addr *net.UDPAddr) (*Message, error) {
	if len(packet) < 5 {
		return nil, fmt.Errorf("packet too short")
	}
	if packet[0] != NormalPacket {
		return nil, fmt.Errorf("packet is not normal")
	}

	messageID := binary.LittleEndian.Uint16(packet[1:3])
	index := packet[3]
	total := packet[4]
	payload := packet[5:]

	packetBufferLock.Lock()
	defer packetBufferLock.Unlock()

	msgBuf, exists := packetBuffer[messageID]
	if !exists {
		msgBuf = &ReceivedMessage{
			TotalChunks:  total,
			Chunks:       make([][]byte, total),
			Received:     0,
			Conn:         conn,
			Addr:         addr,
			MissingTries: make(map[byte]int),
			LastRequest:  make(map[byte]time.Time),
		}
		packetBuffer[messageID] = msgBuf
	}

	if msgBuf.Chunks[index] == nil {
		msgBuf.Chunks[index] = payload
		msgBuf.Received++
	}

	msgBuf.LastUpdate = time.Now()

	if msgBuf.Received < total {
		return nil, nil // beklenmeyen parçalar var
	}

	// Tüm parçalar alındı → birleştir
	fullData := bytes.Join(msgBuf.Chunks, nil)
	delete(packetBuffer, messageID)

	return DecodeRawMessage(fullData)
}

const (
	MaxRetryPerChunk = 3
	RetryCooldown    = 1 * time.Second
)

func CheckAndRequestMissingPackets() {
	packetBufferLock.RLock()
	defer packetBufferLock.RUnlock()

	now := time.Now()

	for messageID, msg := range packetBuffer {
		if msg.Received >= msg.TotalChunks {
			continue
		}
		if now.Sub(msg.LastUpdate) < packetTimeout {
			continue
		}

		// Eksik index'leri topla
		var missing []byte
		for i := 0; i < int(msg.TotalChunks); i++ {
			if msg.Chunks[i] != nil {
				continue
			}

			bi := byte(i)

			// Retry sınırı
			if msg.MissingTries[bi] >= MaxRetryPerChunk {
				continue
			}

			// Retry aralığı
			if last, ok := msg.LastRequest[bi]; ok && now.Sub(last) < RetryCooldown {
				continue
			}

			missing = append(missing, bi)
			msg.MissingTries[bi]++
			msg.LastRequest[bi] = now
		}

		if len(missing) > 0 {
			log.Printf("Requesting resend: messageID=%d, missing=%v", messageID, missing)
			sendResendRequest(msg.Conn, msg.Addr, messageID, missing)
		}
	}
}

func sendResendRequest(conn *net.UDPConn, addr *net.UDPAddr, messageID uint16, missingIndexes []byte) {
	buf := new(bytes.Buffer)

	// Custom control type: 0xFF → özel kontrol mesajı
	buf.WriteByte(ResendPacket)
	binary.Write(buf, binary.LittleEndian, messageID)

	buf.WriteByte(byte(len(missingIndexes)))
	buf.Write(missingIndexes)

	conn.WriteToUDP(buf.Bytes(), addr)
}

func HandleResendRequest(packet []byte, sentMessages map[uint16]*SentMessage, conn *net.UDPConn, addr *net.UDPAddr) {
	if len(packet) < 4 || packet[0] != ResendPacket {
		return // geçerli değil
	}

	messageID := binary.LittleEndian.Uint16(packet[1:3])
	count := packet[3]

	if int(4+count) > len(packet) {
		return
	}

	missing := packet[4 : 4+count]

	sentMessage, ok := sentMessages[messageID]
	if !ok {
		log.Printf("No saved message for resend: %d", messageID)
		return
	}

	for _, index := range missing {
		if int(index) >= len(sentMessage.Chunks) {
			continue
		}
		packet := new(bytes.Buffer)
		packet.WriteByte(NormalPacket)
		binary.Write(packet, binary.LittleEndian, messageID)
		packet.WriteByte(index)
		packet.WriteByte(byte(len(sentMessage.Chunks)))
		packet.Write(sentMessage.Chunks[index])

		conn.WriteToUDP(packet.Bytes(), addr)
	}
}
