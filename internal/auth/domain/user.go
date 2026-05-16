package domain

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID              uuid.UUID
	Email           *string
	Phone           *string
	PasswordHash    *string
	Role            Role
	Status          UserStatus
	EmailVerifiedAt *time.Time
	PhoneVerifiedAt *time.Time
	Name            string
	AvatarURL       *string
	Bio             *string
	LastLoginAt     *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       *time.Time
}

func (u *User) HasPassword() bool { return u.PasswordHash != nil && *u.PasswordHash != "" }
func (u *User) IsActive() bool    { return u.Status == StatusActive && u.DeletedAt == nil }

type RefreshSession struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	TokenHash  string
	UserAgent  string
	IP         string
	ExpiresAt  time.Time
	RevokedAt  *time.Time
	CreatedAt  time.Time
}

type SocialAccount struct {
	ID             uuid.UUID
	UserID         uuid.UUID
	Provider       OAuthProvider
	ProviderUserID string
	Email          *string
	CreatedAt      time.Time
}
