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
	buf.WriteByte(uint8(len(m.Players)))
	binary.Write(buf, binary.LittleEndian, uint16(len(playerBytes)))
	binary.Write(buf, binary.LittleEndian, playerBytes)

	// Countries
	countryBytes := make([]byte, 0)
	for _, country := range m.Countries {
		countryByte := EncodeCountry(&country)
		countryBytes = append(countryBytes, countryByte...)
	}
	buf.WriteByte(uint8(len(m.Countries)))
	binary.Write(buf, binary.LittleEndian, uint16(len(countryBytes)))
	binary.Write(buf, binary.LittleEndian, countryBytes)

	buf.WriteByte(uint8(m.OnlineCount))

	return buf.Bytes(), nil
}
