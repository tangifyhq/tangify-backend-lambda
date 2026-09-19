package weborders

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
)

const TableNameWebAddresses = "tangify-web-addresses"

type WebAddress struct {
	UserID         string  `json:"user_id"`
	AddressID      string  `json:"address_id"`
	Label          string  `json:"label"`
	HouseFlat      string  `json:"house_flat"`
	BuildingStreet string  `json:"building_street"`
	Pincode        string  `json:"pincode"`
	ReceiverName   string  `json:"receiver_name"`
	ReceiverPhone  string  `json:"receiver_phone"`
	Lat            float64 `json:"lat,omitempty"`
	Lng            float64 `json:"lng,omitempty"`
	PinLabel       string  `json:"pin_label,omitempty"`
	UpdatedAt      int64   `json:"updated_at"`
}

type UpsertWebAddressRequest struct {
	AddressID      string   `json:"address_id,omitempty"`
	Label          string   `json:"label"`
	HouseFlat      string   `json:"house_flat"`
	BuildingStreet string   `json:"building_street"`
	Pincode        string   `json:"pincode"`
	ReceiverName   string   `json:"receiver_name"`
	ReceiverPhone  string   `json:"receiver_phone"`
	Lat            *float64 `json:"lat,omitempty"`
	Lng            *float64 `json:"lng,omitempty"`
	PinLabel       string   `json:"pin_label,omitempty"`
}

type WebAddressRepository struct {
	db *dynamodb.Client
}

func NewWebAddressRepository(db *dynamodb.Client) *WebAddressRepository {
	return &WebAddressRepository{db: db}
}

func (r *WebAddressRepository) ListByUser(ctx context.Context, userID string) ([]WebAddress, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("web address store is not configured")
	}
	uid := strings.TrimSpace(userID)
	if uid == "" {
		return nil, fmt.Errorf("user_id required")
	}
	out, err := r.db.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(TableNameWebAddresses),
		KeyConditionExpression: aws.String("user_id = :u"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":u": &types.AttributeValueMemberS{Value: uid},
		},
	})
	if err != nil {
		return nil, err
	}
	list := make([]WebAddress, 0, len(out.Items))
	for _, item := range out.Items {
		a, err := decodeWebAddress(item)
		if err != nil {
			continue
		}
		list = append(list, *a)
	}
	// Newest first.
	for i := 0; i < len(list); i++ {
		for j := i + 1; j < len(list); j++ {
			if list[j].UpdatedAt > list[i].UpdatedAt {
				list[i], list[j] = list[j], list[i]
			}
		}
	}
	return list, nil
}

func (r *WebAddressRepository) Get(ctx context.Context, userID, addressID string) (*WebAddress, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("web address store is not configured")
	}
	uid := strings.TrimSpace(userID)
	aid := strings.TrimSpace(addressID)
	if uid == "" || aid == "" {
		return nil, fmt.Errorf("user_id and address_id required")
	}
	out, err := r.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(TableNameWebAddresses),
		Key: map[string]types.AttributeValue{
			"user_id":    &types.AttributeValueMemberS{Value: uid},
			"address_id": &types.AttributeValueMemberS{Value: aid},
		},
	})
	if err != nil {
		return nil, err
	}
	if len(out.Item) == 0 {
		return nil, nil
	}
	return decodeWebAddress(out.Item)
}

func (r *WebAddressRepository) Put(ctx context.Context, address *WebAddress) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("web address store is not configured")
	}
	if address == nil {
		return fmt.Errorf("address required")
	}
	item, err := encodeWebAddress(address)
	if err != nil {
		return err
	}
	_, err = r.db.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(TableNameWebAddresses),
		Item:      item,
	})
	return err
}

func (r *WebAddressRepository) Delete(ctx context.Context, userID, addressID string) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("web address store is not configured")
	}
	uid := strings.TrimSpace(userID)
	aid := strings.TrimSpace(addressID)
	if uid == "" || aid == "" {
		return fmt.Errorf("user_id and address_id required")
	}
	_, err := r.db.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(TableNameWebAddresses),
		Key: map[string]types.AttributeValue{
			"user_id":    &types.AttributeValueMemberS{Value: uid},
			"address_id": &types.AttributeValueMemberS{Value: aid},
		},
	})
	return err
}

type WebAddressService struct {
	repo *WebAddressRepository
}

func NewWebAddressService(repo *WebAddressRepository) *WebAddressService {
	return &WebAddressService{repo: repo}
}

func (s *WebAddressService) List(ctx context.Context, userID string) ([]WebAddress, error) {
	return s.repo.ListByUser(ctx, userID)
}

func (s *WebAddressService) Upsert(ctx context.Context, userID string, req UpsertWebAddressRequest) (*WebAddress, error) {
	uid := strings.TrimSpace(userID)
	if uid == "" {
		return nil, fmt.Errorf("user_id required")
	}
	label := strings.TrimSpace(req.Label)
	house := strings.TrimSpace(req.HouseFlat)
	street := strings.TrimSpace(req.BuildingStreet)
	if label == "" || house == "" || street == "" {
		return nil, fmt.Errorf("label, house_flat, and building_street are required")
	}
	phone := digitsOnly(req.ReceiverPhone)
	if len(phone) > 10 {
		phone = phone[len(phone)-10:]
	}
	now := time.Now().Unix()

	aid := strings.TrimSpace(req.AddressID)
	if aid == "" {
		// Match existing by label (case-insensitive) so "Home" updates in place.
		existing, err := s.repo.ListByUser(ctx, uid)
		if err != nil {
			return nil, err
		}
		labelKey := strings.ToLower(label)
		for _, a := range existing {
			if strings.ToLower(a.Label) == labelKey {
				aid = a.AddressID
				break
			}
		}
	}
	if aid == "" {
		aid = uuid.NewString()
	} else {
		// Ensure the address belongs to this user when updating by id.
		cur, err := s.repo.Get(ctx, uid, aid)
		if err != nil {
			return nil, err
		}
		if cur == nil {
			// Allow client-supplied ids (e.g. migrated from localStorage).
		}
	}

	addr := &WebAddress{
		UserID:         uid,
		AddressID:      aid,
		Label:          label,
		HouseFlat:      house,
		BuildingStreet: street,
		Pincode:        digitsOnly(req.Pincode),
		ReceiverName:   strings.TrimSpace(req.ReceiverName),
		ReceiverPhone:  phone,
		PinLabel:       strings.TrimSpace(req.PinLabel),
		UpdatedAt:      now,
	}
	if req.Lat != nil {
		addr.Lat = *req.Lat
	}
	if req.Lng != nil {
		addr.Lng = *req.Lng
	}
	if err := s.repo.Put(ctx, addr); err != nil {
		return nil, err
	}
	return addr, nil
}

func (s *WebAddressService) Delete(ctx context.Context, userID, addressID string) error {
	cur, err := s.repo.Get(ctx, userID, addressID)
	if err != nil {
		return err
	}
	if cur == nil {
		return fmt.Errorf("address not found")
	}
	return s.repo.Delete(ctx, userID, addressID)
}

func encodeWebAddress(a *WebAddress) (map[string]types.AttributeValue, error) {
	if a == nil || a.UserID == "" || a.AddressID == "" {
		return nil, fmt.Errorf("invalid address")
	}
	item := map[string]types.AttributeValue{
		"user_id":         &types.AttributeValueMemberS{Value: a.UserID},
		"address_id":      &types.AttributeValueMemberS{Value: a.AddressID},
		"label":           &types.AttributeValueMemberS{Value: a.Label},
		"house_flat":      &types.AttributeValueMemberS{Value: a.HouseFlat},
		"building_street": &types.AttributeValueMemberS{Value: a.BuildingStreet},
		"pincode":         &types.AttributeValueMemberS{Value: a.Pincode},
		"receiver_name":   &types.AttributeValueMemberS{Value: a.ReceiverName},
		"receiver_phone":  &types.AttributeValueMemberS{Value: a.ReceiverPhone},
		"updated_at":      &types.AttributeValueMemberN{Value: strconv.FormatInt(a.UpdatedAt, 10)},
	}
	if a.PinLabel != "" {
		item["pin_label"] = &types.AttributeValueMemberS{Value: a.PinLabel}
	}
	if a.Lat != 0 {
		item["lat"] = &types.AttributeValueMemberN{Value: strconv.FormatFloat(a.Lat, 'f', -1, 64)}
	}
	if a.Lng != 0 {
		item["lng"] = &types.AttributeValueMemberN{Value: strconv.FormatFloat(a.Lng, 'f', -1, 64)}
	}
	return item, nil
}

func decodeWebAddress(item map[string]types.AttributeValue) (*WebAddress, error) {
	a := &WebAddress{}
	if v, ok := item["user_id"].(*types.AttributeValueMemberS); ok {
		a.UserID = v.Value
	}
	if v, ok := item["address_id"].(*types.AttributeValueMemberS); ok {
		a.AddressID = v.Value
	}
	if a.UserID == "" || a.AddressID == "" {
		return nil, fmt.Errorf("invalid address item")
	}
	if v, ok := item["label"].(*types.AttributeValueMemberS); ok {
		a.Label = v.Value
	}
	if v, ok := item["house_flat"].(*types.AttributeValueMemberS); ok {
		a.HouseFlat = v.Value
	}
	if v, ok := item["building_street"].(*types.AttributeValueMemberS); ok {
		a.BuildingStreet = v.Value
	}
	if v, ok := item["pincode"].(*types.AttributeValueMemberS); ok {
		a.Pincode = v.Value
	}
	if v, ok := item["receiver_name"].(*types.AttributeValueMemberS); ok {
		a.ReceiverName = v.Value
	}
	if v, ok := item["receiver_phone"].(*types.AttributeValueMemberS); ok {
		a.ReceiverPhone = v.Value
	}
	if v, ok := item["pin_label"].(*types.AttributeValueMemberS); ok {
		a.PinLabel = v.Value
	}
	if v, ok := item["updated_at"].(*types.AttributeValueMemberN); ok {
		a.UpdatedAt, _ = strconv.ParseInt(v.Value, 10, 64)
	}
	if v, ok := item["lat"].(*types.AttributeValueMemberN); ok {
		a.Lat, _ = strconv.ParseFloat(v.Value, 64)
	}
	if v, ok := item["lng"].(*types.AttributeValueMemberN); ok {
		a.Lng, _ = strconv.ParseFloat(v.Value, 64)
	}
	return a, nil
}
