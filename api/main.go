package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	"tangify-backend-lambda/billing"
	"tangify-backend-lambda/menu"
)

func getJwtClaims(jwtToken string, jwtSecret string) (*MyClaims, error) {
	jwtUtils := NewJwtUtils(jwtSecret)
	claims, err := jwtUtils.ParseJWT(jwtToken)
	if err != nil {
		return nil, err
	}
	return claims, nil
}

var whitelistedRoutes = []string{
	"/api/v1/auth/login",
	"/api/v1/menu",
	"/api/v1/health",
}

type AppContext struct {
	JWTClaims *MyClaims
}

func NewAppContext(claims *MyClaims) *AppContext {
	return &AppContext{
		JWTClaims: claims,
	}
}

func doJwtAuth(request events.LambdaFunctionURLRequest, jwtSecret string, appContext *AppContext) error {
	token := strings.TrimPrefix(request.Headers["authorization"], "Bearer ")
	if token == "" {
		fmt.Println("missing JWT")
		return ErrMissingJWT
	}
	claims, err := getJwtClaims(token, jwtSecret)
	if err != nil {
		fmt.Println("error parsing JWT: ", err)
		return ErrInvalidJWT
	}

	appContext.JWTClaims = claims

	return nil
}

func handler(ctx context.Context, request events.LambdaFunctionURLRequest) (events.LambdaFunctionURLResponse, error) {
	awsUtils := NewAwsUtils()
	route := request.RawPath
	method := request.RequestContext.HTTP.Method
	fmt.Println("method & route: ", method, route)
	appContext := NewAppContext(nil)
	commonUtils := NewCommonUtils()

	// if route not in whitelist, do jwt auth
	if !slices.Contains(whitelistedRoutes, route) {
		jwtSecret, err := awsUtils.GetSSMParameter(ctx, "tangify.jwt.secret")
		if err != nil {
			fmt.Println("error getting JWT secret: ", err)
			return ApiResponse.Error(http.StatusInternalServerError, "Server error: Failed to get JWT secret"), nil
		}

		err = doJwtAuth(request, jwtSecret, appContext)
		if err != nil {
			return ApiResponse.Unauthorized(fmt.Sprintf("Unauthorized: %v", err)), nil
		}
	}

	if method == "GET" && route == "/health" {
		return ApiResponse.Success(map[string]string{"status": "ok"}), nil
	}

	// GET /menu - fetch menu items from Google Sheets
	if method == "GET" && route == "/api/v1/menu" {
		apiKey := os.Getenv("GOOGLE_SHEETS_API_KEY")
		sheetID := os.Getenv("GOOGLE_SHEET_ID")
		sheetName := os.Getenv("GOOGLE_SHEET_NAME")
		if apiKey == "" || sheetID == "" {
			return ApiResponse.Error(http.StatusInternalServerError, "Google Sheets API key or Sheet ID not provided"), nil
		}
		items, err := menu.Fetch(ctx, apiKey, sheetID, sheetName)
		if err != nil {
			fmt.Println("menu fetch error: ", err)
			return ApiResponse.Error(http.StatusInternalServerError, "Failed to fetch data from Google Sheets"), nil
		}
		return ApiResponse.Success(items), nil
	}

	dynamoDBClient, err := awsUtils.GetDynamoDBClient()
	if err != nil {
		fmt.Println("error getting DynamoDB client: ", err)
		return ApiResponse.Error(http.StatusInternalServerError, "Server error: Failed to get DynamoDB client"), nil
	}
	billingService := billing.NewService(billing.NewRepository(dynamoDBClient))

	// create order
	if method == "POST" && route == "/api/v1/billing/order" {
		var addOrderRequest billing.AddOrderRequestType
		err = json.Unmarshal([]byte(request.Body), &addOrderRequest)
		if err != nil {
			fmt.Println("error validating add order request: ", err)
			return ApiResponse.BadRequest("Invalid add order request"), nil
		}
		staffID := fmt.Sprintf("%s::%s", appContext.JWTClaims.Name, appContext.JWTClaims.Identity)

		prefixOrder := billing.PrefixOrder
		orderID := commonUtils.GenerateUniqueID(&prefixOrder)
		totalPrice := int64(0)
		readyAt := int64(0)
		completedAt := int64(0)
		updatedAt := commonUtils.GetCurrentTimestamp()
		pending := billing.PaymentStatusPending
		bill, err := billingService.AddOrderToBill(ctx, addOrderRequest.BillID, &billing.OrderType{
			ID:          &orderID,
			Items:       &addOrderRequest.Items,
			TableID:     addOrderRequest.TableID,
			CustomerID:  addOrderRequest.CustomerID,
			StaffID:     &staffID,
			TotalPrice:  &totalPrice,
			Status:      &pending,
			OrderedAt:   addOrderRequest.OrderedAt,
			ReadyAt:     &readyAt,
			CompletedAt: &completedAt,
			UpdatedAt:   &updatedAt,
		}, commonUtils)
		if err != nil {
			fmt.Println("error adding order to bill: ", err)
			return ApiResponse.Error(http.StatusInternalServerError, "Error adding order to bill: "+err.Error()), nil
		}
		return ApiResponse.Success(bill), nil
	}

	return ApiResponse.Success(map[string]string{"message": "Hello, World!"}), nil
}

func main() {
	lambda.Start(handler)
}
