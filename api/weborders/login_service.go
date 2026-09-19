package weborders

import (
	"context"
	"fmt"

	"tangify-backend-lambda/users"
)

type CustomerTokenIssuer interface {
	CreateOrGetCustomer(ctx context.Context, phone, name string, now int64) (*users.UserPublic, error)
	IssueCustomerToken(userID, name string) (string, error)
}

type LoginService struct {
	repo  *LoginNonceRepository
	users CustomerTokenIssuer
}

func NewLoginService(repo *LoginNonceRepository, users CustomerTokenIssuer) *LoginService {
	return &LoginService{repo: repo, users: users}
}

// StartFromWhatsApp creates a one-time nonce after a "login: web" inbound message.
func (s *LoginService) StartFromWhatsApp(ctx context.Context, senderPhone string, nowMs int64) (string, error) {
	if s == nil || s.users == nil || s.repo == nil {
		return "", fmt.Errorf("web login is not configured")
	}
	user, err := s.users.CreateOrGetCustomer(ctx, senderPhone, "", nowMs)
	if err != nil {
		return "", err
	}
	token, err := s.users.IssueCustomerToken(user.ID, user.Name)
	if err != nil {
		return "", err
	}
	nowSec := nowMs / 1000
	if nowMs < 1_000_000_000_000 {
		nowSec = nowMs
	}
	ttl := nowSec + loginNonceTTLSeconds
	phone := displayPhone(user.Phone)

	for range loginNoncePutRetries {
		nonce, err := generateNonce()
		if err != nil {
			return "", err
		}
		ok, err := s.repo.PutIfAbsent(ctx, &LoginNonce{
			Nonce:  nonce,
			UserID: user.ID,
			JWT:    token,
			Phone:  phone,
			TTL:    ttl,
		})
		if err != nil {
			return "", err
		}
		if ok {
			return nonce, nil
		}
	}
	return "", fmt.Errorf("could not allocate a unique login code")
}

// ConsumeNonce deletes the ticket and returns session fields if still valid.
func (s *LoginService) ConsumeNonce(ctx context.Context, key string, nowMs int64) (*ContinueResponse, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("web login is not configured")
	}
	row, err := s.repo.Consume(ctx, key)
	if err != nil {
		return nil, err
	}
	if row == nil || row.UserID == "" || row.JWT == "" {
		return nil, nil
	}
	nowSec := nowMs / 1000
	if nowMs < 1_000_000_000_000 {
		nowSec = nowMs
	}
	if row.TTL > 0 && row.TTL < nowSec {
		return nil, nil
	}
	return &ContinueResponse{
		UserID: row.UserID,
		JWT:    row.JWT,
		Phone:  row.Phone,
	}, nil
}
