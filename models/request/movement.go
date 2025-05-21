package request

type MovementRequest struct {
	TargetX int `json:"target_x"`
	TargetY int `json:"target_y"`
}
