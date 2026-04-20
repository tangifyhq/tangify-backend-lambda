package users

const (
	RoleWaiter  = "waiter"
	RoleKitchen = "kitchen"
	RoleAdmin   = "admin"
)

// TableNameUsers is the DynamoDB table for user accounts.
const TableNameUsers = "tangify_users"

// User is the persisted record (includes password hash; never expose in JSON).
type User struct {
	ID        string `json:"id"`
	Phone     string `json:"phone"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	Role      string `json:"role"`
	PwSalt    string `json:"-"` // random per user; bcrypt input is derived from password + salt
	PwHash    string `json:"-"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

// UserPublic is returned from APIs (no secrets).
type UserPublic struct {
	ID    string `json:"id"`
	Phone string `json:"phone"`
	Email string `json:"email"`
	Name  string `json:"name"`
	Role  string `json:"role"`
}

func (u *User) Public() UserPublic {
	if u == nil {
		return UserPublic{}
	}
	return UserPublic{
		ID: u.ID, Phone: u.Phone, Email: u.Email, Name: u.Name, Role: u.Role,
	}
}

type CreateUserRequest struct {
	Phone    string `json:"phone"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	Role     string `json:"role"`
	Password string `json:"password"`
}

type LoginRequest struct {
	Login    string `json:"login"` // phone or email (normalized match)
	Password string `json:"password"`
}

type LoginResponse struct {
	Token string     `json:"token"`
	User  UserPublic `json:"user"`
}

type ChangePasswordRequest struct {
	UserID          string `json:"user_id"`
	CurrentPassword string `json:"current_password,omitempty"`
	NewPassword     string `json:"new_password"`
}

type BootstrapUserRequest struct {
	Phone    string `json:"phone"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	Role     string `json:"role"`
	Password string `json:"password"`
}
