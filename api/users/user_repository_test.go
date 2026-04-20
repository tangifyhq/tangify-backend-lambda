package users

import (
	"testing"
	"time"
)

func TestEncodeUser_OmitsEmptyEmailForPhoneOnly(t *testing.T) {
	u := &User{
		ID:        "id-1",
		Phone:     "+919439831236",
		Email:     "",
		Name:      "Test",
		Role:      RoleAdmin,
		PwSalt:    "s",
		PwHash:    "h",
		CreatedAt: time.Now().Unix(),
		UpdatedAt: time.Now().Unix(),
	}
	item, err := encodeUser(u)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := item["email"]; ok {
		t.Fatal("email must be omitted when empty (GSI_Email cannot index empty string)")
	}
	if _, ok := item["phone"]; !ok {
		t.Fatal("phone must be present when set")
	}
}

func TestEncodeUser_OmitsEmptyPhoneForEmailOnly(t *testing.T) {
	u := &User{
		ID:        "id-2",
		Phone:     "",
		Email:     "a@b.com",
		Name:      "Test",
		Role:      RoleWaiter,
		PwSalt:    "s",
		PwHash:    "h",
		CreatedAt: time.Now().Unix(),
		UpdatedAt: time.Now().Unix(),
	}
	item, err := encodeUser(u)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := item["phone"]; ok {
		t.Fatal("phone must be omitted when empty (GSI_Phone cannot index empty string)")
	}
	if _, ok := item["email"]; !ok {
		t.Fatal("email must be present when set")
	}
}
