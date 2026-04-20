package users

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
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

const pwSaltBytes = 16

// newPwSalt returns a URL-safe base64 (no padding) random salt stored per user.
func newPwSalt() (string, error) {
	b := make([]byte, pwSaltBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// bcryptInput is the bytes bcrypt hashes. We use password+salt; if longer than bcrypt's
// 72-byte limit, we SHA256 the concatenation first so long passwords still work.
func bcryptInput(password, salt string) []byte {
	if salt == "" {
		return []byte(password)
	}
	combined := password + salt
	if len(combined) <= 72 {
		return []byte(combined)
	}
	sum := sha256.Sum256([]byte(combined))
	return sum[:]
}

func hashPassword(password, salt string) (string, error) {
	b, err := bcrypt.GenerateFromPassword(bcryptInput(password, salt), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// checkPassword verifies password against stored hash. Empty salt uses legacy bcrypt(password) only.
func checkPassword(hash, salt, password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), bcryptInput(password, salt))
	return err == nil
}

// CreateUser stores a new user. Caller must enforce admin authorization.
func (s *Service) CreateUser(ctx context.Context, req CreateUserRequest, now int64) (*UserPublic, error) {
	req.Phone = NormalizePhone(req.Phone)
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Phone == "" && req.Email == "" {
		return nil, fmt.Errorf("either phone or email is required")
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
	if req.Email != "" {
		if u, _ := s.repo.GetUserByEmail(ctx, req.Email); u != nil {
			return nil, fmt.Errorf("email already registered")
		}
	}
	if req.Phone != "" {
		if u, _ := s.repo.GetUserByPhone(ctx, req.Phone); u != nil {
			return nil, fmt.Errorf("phone already registered")
		}
	}
	salt, err := newPwSalt()
	if err != nil {
		return nil, err
	}
	h, err := hashPassword(req.Password, salt)
	if err != nil {
		return nil, err
	}
	u := &User{
		ID:        uuid.NewString(),
		Phone:     req.Phone,
		Email:     req.Email,
		Name:      strings.TrimSpace(req.Name),
		Role:      req.Role,
		PwSalt:    salt,
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
	if u == nil || !checkPassword(u.PwHash, u.PwSalt, req.Password) {
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
		if req.CurrentPassword == "" || !checkPassword(u.PwHash, u.PwSalt, req.CurrentPassword) {
			return fmt.Errorf("current password required or invalid")
		}
	}
	salt, err := newPwSalt()
	if err != nil {
		return err
	}
	h, err := hashPassword(req.NewPassword, salt)
	if err != nil {
		return err
	}
	u.PwSalt = salt
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
