package request

type ChunkRequest struct {
	Chunks []Chunk `json:"chunks"`
}

type Chunk struct {
	ChunkX int `json:"chunk_x"`
	ChunkY int `json:"chunk_y"`
}
