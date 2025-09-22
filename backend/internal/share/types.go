package share

import "time"

type CreateShareReq struct {
	Title       string     `json:"title"`
	Description string     `json:"description"`
	ExpiresAt   *time.Time `json:"expiresAt"`
}

type ShareToUserReq struct {
	TargetUserEmail string `json:"targetUserEmail" binding:"required,email"`
	Permission      string `json:"permission"`
}
