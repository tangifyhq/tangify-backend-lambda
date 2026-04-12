package users

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type Service struct {
	repo       *Repository
	issueToken func(userID, name, role string) (string, error)
}

func NewService(repo *Repository, issueToken func(userID, name, role string) (string, error)) *Service {
	return &Service{repo: repo, issueToken: issueToken}
}

func validRole(r string) bool {
	switch r {
	case RoleWaiter, RoleKitchen, RoleAdmin:
		return true
	default:
		return false
	}
}

func hashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func checkPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

// CreateUser stores a new user. Caller must enforce admin authorization.
func (s *Service) CreateUser(ctx context.Context, req CreateUserRequest, now int64) (*UserPublic, error) {
	req.Phone = NormalizePhone(req.Phone)
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Phone == "" || req.Email == "" {
		return nil, fmt.Errorf("phone and email are required")
	}
	if req.Name == "" || req.Password == "" {
		return nil, fmt.Errorf("name and password are required")
	}
	if !validRole(req.Role) {
		return nil, fmt.Errorf("invalid role")
	}
	if len(req.Password) < 8 {
		return nil, fmt.Errorf("password must be at least 8 characters")
	}
	if u, _ := s.repo.GetUserByEmail(ctx, req.Email); u != nil {
		return nil, fmt.Errorf("email already registered")
	}
	if u, _ := s.repo.GetUserByPhone(ctx, req.Phone); u != nil {
		return nil, fmt.Errorf("phone already registered")
	}
	h, err := hashPassword(req.Password)
	if err != nil {
		return nil, err
	}
	u := &User{
		ID:        uuid.NewString(),
		Phone:     req.Phone,
		Email:     req.Email,
		Name:      strings.TrimSpace(req.Name),
		Role:      req.Role,
		PwHash:    h,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.repo.PutUser(ctx, u); err != nil {
		return nil, err
	}
	p := u.Public()
	return &p, nil
}

// BootstrapFirstUser creates a user when X-Bootstrap-Secret matches env; for initial admin provisioning.
func (s *Service) BootstrapFirstUser(ctx context.Context, req BootstrapUserRequest, now int64) (*UserPublic, error) {
	if req.Role == "" {
		req.Role = RoleAdmin
	}
	cr := CreateUserRequest{
		Phone: req.Phone, Email: req.Email, Name: req.Name, Role: req.Role, Password: req.Password,
	}
	return s.CreateUser(ctx, cr, now)
}

// Login authenticates by email or phone and returns a JWT + public user.
func (s *Service) Login(ctx context.Context, req LoginRequest) (*LoginResponse, error) {
	req.Login = strings.TrimSpace(req.Login)
	if req.Login == "" || req.Password == "" {
		return nil, fmt.Errorf("login and password required")
	}
	var u *User
	var err error
	if strings.Contains(req.Login, "@") {
		u, err = s.repo.GetUserByEmail(ctx, req.Login)
	} else {
		u, err = s.repo.GetUserByPhone(ctx, req.Login)
	}
	if err != nil {
		return nil, err
	}
	if u == nil || !checkPassword(u.PwHash, req.Password) {
		return nil, fmt.Errorf("invalid credentials")
	}
	token, err := s.issueToken(u.ID, u.Name, u.Role)
	if err != nil {
		return nil, err
	}
	pub := u.Public()
	return &LoginResponse{Token: token, User: pub}, nil
}

// ChangePassword: admin may reset anyone without current password; users changing self must send current password.
func (s *Service) ChangePassword(ctx context.Context, actorID, actorRole string, req ChangePasswordRequest, now int64) error {
	if req.UserID == "" || req.NewPassword == "" {
		return fmt.Errorf("user_id and new_password required")
	}
	if len(req.NewPassword) < 8 {
		return fmt.Errorf("new_password must be at least 8 characters")
	}
	u, err := s.repo.GetUserByID(ctx, req.UserID)
	if err != nil {
		return err
	}
	if u == nil {
		return fmt.Errorf("user not found")
	}
	isAdmin := actorRole == RoleAdmin
	isSelf := actorID == req.UserID
	if !isAdmin && !isSelf {
		return fmt.Errorf("forbidden")
	}
	if isSelf && !isAdmin {
		if req.CurrentPassword == "" || !checkPassword(u.PwHash, req.CurrentPassword) {
			return fmt.Errorf("current password required or invalid")
		}
	}
	h, err := hashPassword(req.NewPassword)
	if err != nil {
		return err
	}
	u.PwHash = h
	u.UpdatedAt = now
	return s.repo.PutUser(ctx, u)
}

// GetByID returns the user without password hash.
func (s *Service) GetByID(ctx context.Context, id string) (*UserPublic, error) {
	u, err := s.repo.GetUserByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, nil
	}
	p := u.Public()
	return &p, nil
}
