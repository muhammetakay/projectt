package request

type MovementRequest struct {
	TargetX uint16 `json:"target_x"`
	TargetY uint16 `json:"target_y"`
}
