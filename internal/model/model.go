package model

import "time"

type User struct {
	ID          string    `json:"id"`
	Username    string    `json:"username"`
	Email       string    `json:"-"`
	Password    string    `json:"-"`
	DisplayName string    `json:"display_name"`
	Bio         string    `json:"bio"`
	AvatarURL   string    `json:"avatar_url"`
	Role        string    `json:"role"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// PublicProfile returns a JSON-safe map that excludes sensitive fields.
// Use this for public-facing API responses (profile, search, tweets).
func (u *User) PublicProfile() map[string]any {
	return map[string]any{
		"id":           u.ID,
		"username":     u.Username,
		"display_name": u.DisplayName,
		"bio":          u.Bio,
		"avatar_url":   u.AvatarURL,
		"role":         u.Role,
		"created_at":   u.CreatedAt,
		"updated_at":   u.UpdatedAt,
	}
}

// OwnProfile returns a JSON-safe map that includes email (for the authenticated user's own data).
func (u *User) OwnProfile() map[string]any {
	p := u.PublicProfile()
	p["email"] = u.Email
	return p
}

type Tweet struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Content   string    `json:"content"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`

	// Joined fields (not stored directly)
	User      *User `json:"user,omitempty"`
	CanDelete bool  `json:"-"`
}

type UserStats struct {
	UserID         string `json:"user_id"`
	FollowerCount  int    `json:"follower_count"`
	FollowingCount int    `json:"following_count"`
	TweetCount     int    `json:"tweet_count"`
}

type OutboxEvent struct {
	ID        int64     `json:"id"`
	EventType string    `json:"event_type"`
	Payload   string    `json:"payload"`
	Status    string    `json:"status"`
	Attempts  int       `json:"attempts"`
	CreatedAt time.Time `json:"created_at"`
}

type RefreshToken struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

type AgentAction struct {
	ID         string    `json:"id"`
	AgentID    string    `json:"agent_id"`
	ActionType string    `json:"action_type"`
	TargetType string    `json:"target_type"`
	TargetID   string    `json:"target_id"`
	Reasoning  string    `json:"reasoning"`
	Reversible bool      `json:"reversible"`
	Reversed   bool      `json:"reversed"`
	CreatedAt  time.Time `json:"created_at"`
}
