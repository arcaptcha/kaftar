package dto

type Error struct {
	Status  int            `json:"status"`
	Message string         `json:"message"`
	Details map[string]any `json:"details"`
}
