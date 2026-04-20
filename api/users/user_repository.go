package users

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

const (
	gsiEmail = "GSI_Email"
	gsiPhone = "GSI_Phone"
)

type Repository struct {
	db *dynamodb.Client
}

func NewRepository(db *dynamodb.Client) *Repository {
	return &Repository{db: db}
}

func (r *Repository) PutUser(ctx context.Context, u *User) error {
	if u == nil {
		return fmt.Errorf("user is nil")
	}
	item, err := encodeUser(u)
	if err != nil {
		return err
	}
	_, err = r.db.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(TableNameUsers),
		Item:      item,
	})
	return err
}

func (r *Repository) GetUserByID(ctx context.Context, id string) (*User, error) {
	out, err := r.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(TableNameUsers),
		Key: map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: id},
		},
	})
	if err != nil {
		return nil, err
	}
	if len(out.Item) == 0 {
		return nil, nil
	}
	return decodeUser(out.Item)
}

func (r *Repository) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		return nil, nil
	}
	out, err := r.db.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(TableNameUsers),
		IndexName:              aws.String(gsiEmail),
		KeyConditionExpression: aws.String("email = :e"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":e": &types.AttributeValueMemberS{Value: email},
		},
		Limit: aws.Int32(1),
	})
	if err != nil {
		return nil, err
	}
	if len(out.Items) == 0 {
		return nil, nil
	}
	return decodeUser(out.Items[0])
}

func (r *Repository) GetUserByPhone(ctx context.Context, phone string) (*User, error) {
	phone = NormalizePhone(phone)
	if phone == "" {
		return nil, nil
	}
	out, err := r.db.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(TableNameUsers),
		IndexName:              aws.String(gsiPhone),
		KeyConditionExpression: aws.String("phone = :p"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":p": &types.AttributeValueMemberS{Value: phone},
		},
		Limit: aws.Int32(1),
	})
	if err != nil {
		return nil, err
	}
	if len(out.Items) == 0 {
		return nil, nil
	}
	return decodeUser(out.Items[0])
}

// NormalizePhone trims spaces for phone keys (GSI).
func NormalizePhone(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, " ", ""))
}

func encodeUser(u *User) (map[string]types.AttributeValue, error) {
	phone := NormalizePhone(u.Phone)
	email := strings.ToLower(strings.TrimSpace(u.Email))
	m := map[string]types.AttributeValue{
		"id":         &types.AttributeValueMemberS{Value: u.ID},
		"name":       &types.AttributeValueMemberS{Value: u.Name},
		"role":       &types.AttributeValueMemberS{Value: u.Role},
		"pw_salt":    &types.AttributeValueMemberS{Value: u.PwSalt},
		"pw_hash":    &types.AttributeValueMemberS{Value: u.PwHash},
		"created_at": &types.AttributeValueMemberN{Value: strconv.FormatInt(u.CreatedAt, 10)},
		"updated_at": &types.AttributeValueMemberN{Value: strconv.FormatInt(u.UpdatedAt, 10)},
	}
	// GSI_Email / GSI_Phone keys cannot be empty strings; omit the attribute when unused.
	if phone != "" {
		m["phone"] = &types.AttributeValueMemberS{Value: phone}
	}
	if email != "" {
		m["email"] = &types.AttributeValueMemberS{Value: email}
	}
	return m, nil
}

func decodeUser(item map[string]types.AttributeValue) (*User, error) {
	u := &User{}
	if v, ok := item["id"].(*types.AttributeValueMemberS); ok {
		u.ID = v.Value
	}
	if v, ok := item["phone"].(*types.AttributeValueMemberS); ok {
		u.Phone = v.Value
	}
	if v, ok := item["email"].(*types.AttributeValueMemberS); ok {
		u.Email = v.Value
	}
	if v, ok := item["name"].(*types.AttributeValueMemberS); ok {
		u.Name = v.Value
	}
	if v, ok := item["role"].(*types.AttributeValueMemberS); ok {
		u.Role = v.Value
	}
	if v, ok := item["pw_salt"].(*types.AttributeValueMemberS); ok {
		u.PwSalt = v.Value
	}
	if v, ok := item["pw_hash"].(*types.AttributeValueMemberS); ok {
		u.PwHash = v.Value
	}
	u.CreatedAt, _ = numAttr(item, "created_at")
	u.UpdatedAt, _ = numAttr(item, "updated_at")
	return u, nil
}

func numAttr(item map[string]types.AttributeValue, key string) (int64, error) {
	a, ok := item[key].(*types.AttributeValueMemberN)
	if !ok || a == nil {
		return 0, nil
	}
	return strconv.ParseInt(a.Value, 10, 64)
}
