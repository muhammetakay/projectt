package binary

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"slices"
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

	GameConnection any

	Ack chan bool
}

var (
	packetBuffer     = make(map[uint32]*ReceivedMessage)
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

	messageID := binary.LittleEndian.Uint32(packet[1:5])
	index := packet[5]
	total := packet[6]
	payload := packet[7:]

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

	msg, err := DecodeRawMessage(fullData)
	if err == nil {
		if slices.Contains(AckRequiredMessageTypes, msg.Type) {
			sendAckRequest(msgBuf.Conn, msgBuf.Addr, messageID)
		}
	}

	delete(packetBuffer, messageID)
	return msg, err
}

const (
	MaxRetryPerChunk = 3
	RetryCooldown    = 1 * time.Second
)

func CheckAndRequestMissingPackets() {
	packetBufferLock.Lock()
	messagesToCheck := make(map[uint32]*ReceivedMessage)
	for id, msg := range packetBuffer {
		messagesToCheck[id] = msg
	}
	packetBufferLock.Unlock()

	now := time.Now()

	// check missing packets for received messages
	for messageID, msg := range messagesToCheck {
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

func sendResendRequest(conn *net.UDPConn, addr *net.UDPAddr, messageID uint32, missingIndexes []byte) {
	buf := new(bytes.Buffer)

	// Custom control type: 0xFF → özel kontrol mesajı
	buf.WriteByte(ResendPacket)
	binary.Write(buf, binary.LittleEndian, messageID)

	buf.WriteByte(byte(len(missingIndexes)))
	buf.Write(missingIndexes)

	conn.WriteToUDP(buf.Bytes(), addr)
}

func HandleResendRequest(packet []byte, sentMessages map[uint32]*SentMessage, conn *net.UDPConn, addr *net.UDPAddr) {
	if len(packet) < 4 || packet[0] != ResendPacket {
		return // geçerli değil
	}

	messageID := binary.LittleEndian.Uint32(packet[1:5])
	count := packet[5]

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

func HandleAckRequest(packet []byte, sentMessages map[uint32]*SentMessage) {
	if len(packet) < 5 || packet[0] != AckPacket {
		return // geçerli değil
	}

	messageID := binary.LittleEndian.Uint32(packet[1:5])

	msg, ok := sentMessages[messageID]
	if !ok {
		return
	}

	select {
	case msg.Ack <- true:
	default:
	}
}

func sendAckRequest(conn *net.UDPConn, addr *net.UDPAddr, messageID uint32) {
	buf := new(bytes.Buffer)

	buf.WriteByte(AckPacket)
	binary.Write(buf, binary.LittleEndian, messageID)

	conn.WriteToUDP(buf.Bytes(), addr)
}
