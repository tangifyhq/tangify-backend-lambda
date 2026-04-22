package loyalty

import (
	"context"
	"fmt"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type Repository struct {
	db *dynamodb.Client
}

func NewRepository(db *dynamodb.Client) *Repository {
	return &Repository{db: db}
}

func (r *Repository) GetWallet(ctx context.Context, userID string) (*PointsWallet, error) {
	out, err := r.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(TableNamePointsWallet),
		Key: map[string]types.AttributeValue{
			"user_id": &types.AttributeValueMemberS{Value: userID},
		},
	})
	if err != nil {
		return nil, err
	}
	if len(out.Item) == 0 {
		return &PointsWallet{UserID: userID}, nil
	}
	return decodeWallet(out.Item), nil
}

func (r *Repository) PutWallet(ctx context.Context, w *PointsWallet) error {
	if w == nil {
		return fmt.Errorf("wallet is nil")
	}
	item := map[string]types.AttributeValue{
		"user_id":           &types.AttributeValueMemberS{Value: w.UserID},
		"points_balance":    &types.AttributeValueMemberN{Value: strconv.FormatInt(w.PointsBalance, 10)},
		"lifetime_earned":   &types.AttributeValueMemberN{Value: strconv.FormatInt(w.LifetimeEarned, 10)},
		"lifetime_redeemed": &types.AttributeValueMemberN{Value: strconv.FormatInt(w.LifetimeRedeemed, 10)},
		"updated_at":        &types.AttributeValueMemberN{Value: strconv.FormatInt(w.UpdatedAt, 10)},
	}
	_, err := r.db.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(TableNamePointsWallet),
		Item:      item,
	})
	return err
}

func decodeWallet(item map[string]types.AttributeValue) *PointsWallet {
	w := &PointsWallet{}
	if v, ok := item["user_id"].(*types.AttributeValueMemberS); ok {
		w.UserID = v.Value
	}
	w.PointsBalance, _ = numAttr(item, "points_balance")
	w.LifetimeEarned, _ = numAttr(item, "lifetime_earned")
	w.LifetimeRedeemed, _ = numAttr(item, "lifetime_redeemed")
	w.UpdatedAt, _ = numAttr(item, "updated_at")
	return w
}

func numAttr(item map[string]types.AttributeValue, key string) (int64, error) {
	a, ok := item[key].(*types.AttributeValueMemberN)
	if !ok || a == nil {
		return 0, nil
	}
	return strconv.ParseInt(a.Value, 10, 64)
}
