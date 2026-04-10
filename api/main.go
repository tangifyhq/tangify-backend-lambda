package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strconv"
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

func queryParam(request events.LambdaFunctionURLRequest, key string) string {
	if request.QueryStringParameters == nil {
		return ""
	}
	return request.QueryStringParameters[key]
}

func staffIDFromContext(app *AppContext) string {
	if app == nil || app.JWTClaims == nil {
		return ""
	}
	return fmt.Sprintf("%s::%s", app.JWTClaims.Name, app.JWTClaims.Identity)
}

func handler(ctx context.Context, request events.LambdaFunctionURLRequest) (events.LambdaFunctionURLResponse, error) {
	awsUtils := NewAwsUtils()
	route := request.RawPath
	method := request.RequestContext.HTTP.Method
	fmt.Println("method & route: ", method, route)
	appContext := NewAppContext(nil)
	commonUtils := NewCommonUtils()

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

	if method == "GET" && route == "/api/v1/health" {
		return ApiResponse.Success(map[string]string{"status": "ok"}), nil
	}

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
	staffID := staffIDFromContext(appContext)

	// --- Waiter / billing ---
	if method == "GET" && route == "/api/v1/billing/live-orders" {
		data, err := billingService.LiveOrdersGrouped(ctx, queryParam(request, "venue_id"))
		if err != nil {
			return ApiResponse.Error(http.StatusInternalServerError, err.Error()), nil
		}
		return ApiResponse.Success(data), nil
	}

	if method == "POST" && route == "/api/v1/billing/sessions" {
		var body billing.CreateSessionAndFirstOrderRequest
		if err := json.Unmarshal([]byte(request.Body), &body); err != nil {
			return ApiResponse.BadRequest("Invalid JSON body"), nil
		}
		data, err := billingService.CreateSessionAndFirstOrder(ctx, body, staffID, commonUtils)
		if err != nil {
			return ApiResponse.Error(http.StatusBadRequest, err.Error()), nil
		}
		return ApiResponse.Success(data), nil
	}

	if method == "POST" && route == "/api/v1/billing/orders" {
		var body billing.AddOrderToSessionRequest
		if err := json.Unmarshal([]byte(request.Body), &body); err != nil {
			return ApiResponse.BadRequest("Invalid JSON body"), nil
		}
		data, err := billingService.AddOrder(ctx, body, staffID, commonUtils)
		if err != nil {
			return ApiResponse.Error(http.StatusBadRequest, err.Error()), nil
		}
		return ApiResponse.Success(data), nil
	}

	if method == "PATCH" && route == "/api/v1/billing/orders" {
		var body billing.UpdateOrderRequestV2
		if err := json.Unmarshal([]byte(request.Body), &body); err != nil {
			return ApiResponse.BadRequest("Invalid JSON body"), nil
		}
		data, err := billingService.UpdateOrderWithClock(ctx, body, commonUtils)
		if err != nil {
			return ApiResponse.Error(http.StatusBadRequest, err.Error()), nil
		}
		return ApiResponse.Success(data), nil
	}

	if method == "GET" && route == "/api/v1/billing/orders" {
		sid := queryParam(request, "session_id")
		tid := queryParam(request, "table_id")
		if sid != "" {
			data, err := billingService.ListOrdersBySession(ctx, sid)
			if err != nil {
				return ApiResponse.Error(http.StatusInternalServerError, err.Error()), nil
			}
			return ApiResponse.Success(data), nil
		}
		if tid != "" {
			data, err := billingService.ListOrdersByTable(ctx, queryParam(request, "venue_id"), tid)
			if err != nil {
				return ApiResponse.Error(http.StatusInternalServerError, err.Error()), nil
			}
			return ApiResponse.Success(data), nil
		}
		return ApiResponse.BadRequest("session_id or table_id query param required"), nil
	}

	if method == "POST" && route == "/api/v1/billing/bills/start" {
		var body billing.StartBillForSessionRequest
		if err := json.Unmarshal([]byte(request.Body), &body); err != nil {
			return ApiResponse.BadRequest("Invalid JSON body"), nil
		}
		data, err := billingService.StartBill(ctx, body, staffID, commonUtils)
		if err != nil {
			return ApiResponse.Error(http.StatusBadRequest, err.Error()), nil
		}
		return ApiResponse.Success(data), nil
	}

	if method == "PATCH" && route == "/api/v1/billing/bills" {
		var body billing.UpdateBillRequestV2
		if err := json.Unmarshal([]byte(request.Body), &body); err != nil {
			return ApiResponse.BadRequest("Invalid JSON body"), nil
		}
		data, err := billingService.UpdateBill(ctx, body, commonUtils)
		if err != nil {
			return ApiResponse.Error(http.StatusBadRequest, err.Error()), nil
		}
		return ApiResponse.Success(data), nil
	}

	if method == "POST" && route == "/api/v1/billing/sessions/close" {
		var body billing.CloseTableRequest
		if err := json.Unmarshal([]byte(request.Body), &body); err != nil {
			return ApiResponse.BadRequest("Invalid JSON body"), nil
		}
		if err := billingService.CloseTable(ctx, body, commonUtils); err != nil {
			return ApiResponse.Error(http.StatusBadRequest, err.Error()), nil
		}
		return ApiResponse.Success(map[string]string{"status": "closed"}), nil
	}

	// --- Kitchen ---
	if method == "GET" && route == "/api/v1/kitchen/item-board" {
		data, err := billingService.KitchenItemBoard(ctx, queryParam(request, "venue_id"))
		if err != nil {
			return ApiResponse.Error(http.StatusInternalServerError, err.Error()), nil
		}
		return ApiResponse.Success(data), nil
	}

	if method == "PATCH" && route == "/api/v1/kitchen/line-items/status" {
		var body billing.PatchLineItemStatusRequest
		if err := json.Unmarshal([]byte(request.Body), &body); err != nil {
			return ApiResponse.BadRequest("Invalid JSON body"), nil
		}
		data, err := billingService.PatchLineItemStatus(ctx, body, commonUtils)
		if err != nil {
			return ApiResponse.Error(http.StatusBadRequest, err.Error()), nil
		}
		return ApiResponse.Success(data), nil
	}

	// --- Plating ---
	if method == "GET" && route == "/api/v1/plating/orders" {
		limit := 100
		if ls := queryParam(request, "limit"); ls != "" {
			if n, e := strconv.Atoi(ls); e == nil && n > 0 {
				limit = n
			}
		}
		data, err := billingService.PlatingFIFO(ctx, queryParam(request, "venue_id"), queryParam(request, "table_id"), queryParam(request, "session_id"), limit)
		if err != nil {
			return ApiResponse.Error(http.StatusBadRequest, err.Error()), nil
		}
		return ApiResponse.Success(data), nil
	}

	if method == "PATCH" && route == "/api/v1/plating/orders/status" {
		var body billing.PatchOrderKitchenStatusRequestV2
		if err := json.Unmarshal([]byte(request.Body), &body); err != nil {
			return ApiResponse.BadRequest("Invalid JSON body"), nil
		}
		data, err := billingService.PatchOrderKitchenStatus(ctx, body, commonUtils)
		if err != nil {
			return ApiResponse.Error(http.StatusBadRequest, err.Error()), nil
		}
		return ApiResponse.Success(data), nil
	}

	return ApiResponse.Success(map[string]string{"message": "Hello, World!"}), nil
}

func main() {
	lambda.Start(handler)
}
