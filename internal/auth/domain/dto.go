package domain

import "time"

// ── Requests ─────────────────────────────────────────

type RegisterRequest struct {
	Email    string `json:"email" binding:"omitempty,email"`
	Phone    string `json:"phone" binding:"omitempty,e164"`
	Password string `json:"password" binding:"required,strong_password"`
	Name     string `json:"name" binding:"required,min=1,max=120"`
}

type LoginRequest struct {
	// One of Email/Phone is required; validated in service.
	Email    string `json:"email" binding:"omitempty,email"`
	Phone    string `json:"phone" binding:"omitempty,e164"`
	Password string `json:"password" binding:"required"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type LogoutRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type ForgotPasswordRequest struct {
	Email string `json:"email" binding:"omitempty,email"`
	Phone string `json:"phone" binding:"omitempty,e164"`
}

type ResetPasswordRequest struct {
	Email       string `json:"email" binding:"omitempty,email"`
	Phone       string `json:"phone" binding:"omitempty,e164"`
	OTP         string `json:"otp" binding:"required,len=6"`
	NewPassword string `json:"new_password" binding:"required,strong_password"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required,strong_password"`
}

type SendOTPRequest struct {
	Email   string `json:"email" binding:"omitempty,email"`
	Phone   string `json:"phone" binding:"omitempty,e164"`
	Purpose string `json:"purpose" binding:"required,oneof=verify_email verify_phone reset_password"`
}

type VerifyOTPRequest struct {
	Email   string `json:"email" binding:"omitempty,email"`
	Phone   string `json:"phone" binding:"omitempty,e164"`
	OTP     string `json:"otp" binding:"required,len=6"`
	Purpose string `json:"purpose" binding:"required,oneof=verify_email verify_phone reset_password"`
}

type SocialLoginRequest struct {
	// ID token from the provider (Google ID token or Apple identity token).
	IDToken string `json:"id_token" binding:"required"`
}

type UpdateProfileRequest struct {
	Name      *string `json:"name" binding:"omitempty,min=1,max=120"`
	AvatarURL *string `json:"avatar_url" binding:"omitempty,url"`
	Bio       *string `json:"bio" binding:"omitempty,max=500"`
}

type DeleteAccountRequest struct {
	Password string `json:"password" binding:"required"`
}

// ── Responses ────────────────────────────────────────

type AuthTokens struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type AuthResponse struct {
	User   UserResponse `json:"user"`
	Tokens AuthTokens   `json:"tokens"`
}

type UserResponse struct {
	ID            string  `json:"id"`
	Email         *string `json:"email,omitempty"`
	Phone         *string `json:"phone,omitempty"`
	Name          string  `json:"name"`
	Role          string  `json:"role"`
	Status        string  `json:"status"`
	AvatarURL     *string `json:"avatar_url,omitempty"`
	Bio           *string `json:"bio,omitempty"`
	EmailVerified bool    `json:"email_verified"`
	PhoneVerified bool    `json:"phone_verified"`
	CreatedAt     string  `json:"created_at"`
}

func ToUserResponse(u *User) UserResponse {
	return UserResponse{
		ID:            u.ID.String(),
		Email:         u.Email,
		Phone:         u.Phone,
		Name:          u.Name,
		Role:          string(u.Role),
		Status:        string(u.Status),
		AvatarURL:     u.AvatarURL,
		Bio:           u.Bio,
		EmailVerified: u.EmailVerifiedAt != nil,
		PhoneVerified: u.PhoneVerifiedAt != nil,
		CreatedAt:     u.CreatedAt.UTC().Format(time.RFC3339),
	}
}
