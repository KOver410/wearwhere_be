package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/wearwhere/wearwhere_be/internal/auth/domain"
	"github.com/wearwhere/wearwhere_be/internal/auth/repo"
	"github.com/wearwhere/wearwhere_be/internal/shared/hash"
)

type ProfileService struct {
	users    repo.UserRepo
	sessions repo.SessionRepo
}

func NewProfileService(u repo.UserRepo, s repo.SessionRepo) *ProfileService {
	return &ProfileService{users: u, sessions: s}
}

func (p *ProfileService) Get(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	u, err := p.users.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, domain.ErrUserNotFound
		}
		return nil, err
	}
	return u, nil
}

func (p *ProfileService) Update(ctx context.Context, id uuid.UUID, req domain.UpdateProfileRequest) (*domain.User, error) {
	if err := p.users.UpdateProfile(ctx, id, req.Name, req.AvatarURL, req.Bio); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, domain.ErrUserNotFound
		}
		return nil, err
	}
	return p.users.GetByID(ctx, id)
}

// Delete performs the GDPR-compliant soft delete (UC09). Password
// re-confirmation is required to prevent accidental deletion.
// hasPendingOrders is injected by the caller so the auth module doesn't depend
// on the orders module directly.
func (p *ProfileService) Delete(ctx context.Context, id uuid.UUID, password string, hasPendingOrders func(context.Context, uuid.UUID) (bool, error)) error {
	user, err := p.users.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return domain.ErrUserNotFound
		}
		return err
	}
	if !user.HasPassword() || !hash.ComparePassword(*user.PasswordHash, password) {
		return domain.ErrInvalidCredentials
	}
	if hasPendingOrders != nil {
		pending, err := hasPendingOrders(ctx, id)
		if err != nil {
			return err
		}
		if pending {
			return domain.ErrPendingOrders
		}
	}

	var emailHash, phoneHash *string
	if user.Email != nil {
		h := hash.SHA256Hex(*user.Email)
		emailHash = &h
	}
	if user.Phone != nil {
		h := hash.SHA256Hex(*user.Phone)
		phoneHash = &h
	}
	purgeAfter := time.Now().Add(90 * 24 * time.Hour) // SRS UC09: 90 days

	if err := p.users.SoftDelete(ctx, id, emailHash, phoneHash, purgeAfter); err != nil {
		return err
	}
	_ = p.sessions.RevokeAllForUser(ctx, id)
	return nil
}
