package models

import "time"

// UserAdapterToken represents a per-user token for accessing a specific adapter
type UserAdapterToken struct {
	UserID     string     `json:"user_id" example:"user123"`
	AdapterID  string     `json:"adapter_id" example:"adapter-abc"`
	Token      string     `json:"token" example:"eyJhbGciOiJIUzI1NiIs..."`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

// IsExpired checks if the token has expired
func (t *UserAdapterToken) IsExpired() bool {
	if t.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*t.ExpiresAt)
}

// UpdateLastUsed updates the last used timestamp
func (t *UserAdapterToken) UpdateLastUsed() {
	now := time.Now()
	t.LastUsedAt = &now
}
