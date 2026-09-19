package weborders

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type LoginNonceRepository struct {
	db *dynamodb.Client
}

func NewLoginNonceRepository(db *dynamodb.Client) *LoginNonceRepository {
	return &LoginNonceRepository{db: db}
}

func (r *LoginNonceRepository) PutIfAbsent(ctx context.Context, row *LoginNonce) (ok bool, err error) {
	if r == nil || r.db == nil {
		return false, fmt.Errorf("login nonce store is not configured")
	}
	if row == nil || row.Nonce == "" {
		return false, fmt.Errorf("nonce required")
	}
	_, err = r.db.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(TableNameLoginNonce),
		Item: map[string]types.AttributeValue{
			"nonce":   &types.AttributeValueMemberS{Value: row.Nonce},
			"user_id": &types.AttributeValueMemberS{Value: row.UserID},
			"jwt":     &types.AttributeValueMemberS{Value: row.JWT},
			"phone":   &types.AttributeValueMemberS{Value: row.Phone},
			"ttl":     &types.AttributeValueMemberN{Value: strconv.FormatInt(row.TTL, 10)},
		},
		ConditionExpression: aws.String("attribute_not_exists(nonce)"),
	})
	if err != nil {
		var cond *types.ConditionalCheckFailedException
		if errors.As(err, &cond) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (r *LoginNonceRepository) Consume(ctx context.Context, nonce string) (*LoginNonce, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("login nonce store is not configured")
	}
	out, err := r.db.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(TableNameLoginNonce),
		Key: map[string]types.AttributeValue{
			"nonce": &types.AttributeValueMemberS{Value: nonce},
		},
		ReturnValues: types.ReturnValueAllOld,
	})
	if err != nil {
		return nil, err
	}
	if len(out.Attributes) == 0 {
		return nil, nil
	}
	return decodeLoginNonce(out.Attributes), nil
}

func decodeLoginNonce(item map[string]types.AttributeValue) *LoginNonce {
	row := &LoginNonce{}
	if v, ok := item["nonce"].(*types.AttributeValueMemberS); ok {
		row.Nonce = v.Value
	}
	if v, ok := item["user_id"].(*types.AttributeValueMemberS); ok {
		row.UserID = v.Value
	}
	if v, ok := item["jwt"].(*types.AttributeValueMemberS); ok {
		row.JWT = v.Value
	}
	if v, ok := item["phone"].(*types.AttributeValueMemberS); ok {
		row.Phone = v.Value
	}
	if v, ok := item["ttl"].(*types.AttributeValueMemberN); ok {
		n, _ := strconv.ParseInt(v.Value, 10, 64)
		row.TTL = n
	}
	return row
}
