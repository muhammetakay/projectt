package binary

import (
	"bytes"
	"encoding/binary"
)

type SyncStateData struct {
	Players     []*Player
	Countries   []Country
	OnlineCount int
}

func EncodeSyncStateData(m *SyncStateData) ([]byte, error) {
	buf := new(bytes.Buffer)

	// Players
	playerBytes := make([]byte, 0)
	for _, player := range m.Players {
		playerByte, err := EncodePlayer(player)
		if err != nil {
			continue
		}
		playerBytes = append(playerBytes, playerByte...)
	}
	binary.Write(buf, binary.BigEndian, uint16(len(playerBytes)))
	buf.Write(playerBytes)

	// Countries
	countryBytes := make([]byte, 0)
	for _, country := range m.Countries {
		countryByte := EncodeCountry(&country)
		countryBytes = append(playerBytes, countryByte...)
	}
	binary.Write(buf, binary.BigEndian, uint16(len(countryBytes)))
	buf.Write(countryBytes)

	buf.WriteByte(uint8(m.OnlineCount))

	return buf.Bytes(), nil
}
