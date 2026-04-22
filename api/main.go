package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	"tangify-backend-lambda/billing"
	"tangify-backend-lambda/loyalty"
	"tangify-backend-lambda/menu"
	"tangify-backend-lambda/users"
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
	"/api/v1/users/bootstrap",
	"/api/v1/users/customer-onboard",
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

func headerGet(headers map[string]string, key string) string {
	if headers == nil {
		return ""
	}
	lk := strings.ToLower(key)
	for k, v := range headers {
		if strings.ToLower(k) == lk {
			return v
		}
	}
	return ""
}

func handler(ctx context.Context, request events.LambdaFunctionURLRequest) (events.LambdaFunctionURLResponse, error) {
	awsUtils := NewAwsUtils()
	ablyUtils, ablyErr := NewAblyUtils()
	if ablyErr != nil {
		fmt.Println("error initializing Ably client: ", ablyErr)
	}
	route := request.RawPath
	method := request.RequestContext.HTTP.Method
	fmt.Println("method & route: ", method, route)
	appContext := NewAppContext(nil)
	commonUtils := NewCommonUtils()

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

	jwtSecret, err := awsUtils.GetSSMParameter(ctx, "tangify.jwt.secret")
	if err != nil {
		fmt.Println("error getting JWT secret: ", err)
		return ApiResponse.Error(http.StatusInternalServerError, "Server error: Failed to get JWT secret"), nil
	}

	usersService := users.NewService(users.NewRepository(dynamoDBClient), func(userID, name, role string) (string, error) {
		j := NewJwtUtils(jwtSecret)
		return j.GenerateJWT(userID, name, role, 24*time.Hour)
	})
	billRepo := billing.NewRepository(dynamoDBClient)
	loyaltyService := loyalty.NewService(loyalty.NewRepository(dynamoDBClient), billRepo)

	if method == "POST" && route == "/api/v1/auth/login" {
		var body users.LoginRequest
		if err := json.Unmarshal([]byte(request.Body), &body); err != nil {
			return ApiResponse.BadRequest("Invalid JSON body"), nil
		}
		data, err := usersService.Login(ctx, body)
		if err != nil {
			return ApiResponse.Error(http.StatusUnauthorized, err.Error()), nil
		}
		return ApiResponse.Success(data), nil
	}

	if method == "POST" && route == "/api/v1/users/bootstrap" {
		want := strings.TrimSpace(os.Getenv("TANGIFY_BOOTSTRAP_SECRET"))
		if want == "" {
			return ApiResponse.Error(http.StatusForbidden, "Bootstrap is not configured"), nil
		}
		if strings.TrimSpace(headerGet(request.Headers, "X-Bootstrap-Secret")) != want {
			return ApiResponse.Unauthorized("Invalid bootstrap secret"), nil
		}
		var body users.BootstrapUserRequest
		if err := json.Unmarshal([]byte(request.Body), &body); err != nil {
			return ApiResponse.BadRequest("Invalid JSON body"), nil
		}
		data, err := usersService.BootstrapFirstUser(ctx, body, commonUtils.GetCurrentTimestamp())
		if err != nil {
			return ApiResponse.Error(http.StatusBadRequest, err.Error()), nil
		}
		return ApiResponse.Success(data), nil
	}

	if method == "POST" && route == "/api/v1/users/customer-onboard" {
		want := strings.TrimSpace(os.Getenv("CF_SECRET"))
		if want == "" {
			return ApiResponse.Error(http.StatusForbidden, "CF onboarding is not configured"), nil
		}
		if strings.TrimSpace(headerGet(request.Headers, "X-CF-Secret")) != want {
			return ApiResponse.Unauthorized("Invalid CF secret"), nil
		}
		var body users.CustomerOnboardRequest
		if err := json.Unmarshal([]byte(request.Body), &body); err != nil {
			return ApiResponse.BadRequest("Invalid JSON body"), nil
		}
		user, err := usersService.CreateOrGetCustomer(ctx, body.Phone, body.Name, commonUtils.GetCurrentTimestamp())
		if err != nil {
			return ApiResponse.Error(http.StatusBadRequest, err.Error()), nil
		}
		if err := sendGupshupPlaceholderMessage(ctx, user.Phone, user.Name); err != nil {
			return ApiResponse.Error(http.StatusBadGateway, err.Error()), nil
		}
		return ApiResponse.Success(user), nil
	}

	if !slices.Contains(whitelistedRoutes, route) {
		err = doJwtAuth(request, jwtSecret, appContext)
		if err != nil {
			return ApiResponse.Unauthorized(fmt.Sprintf("Unauthorized: %v", err)), nil
		}
	}

	billingService := billing.NewService(billRepo)
	staffID := staffIDFromContext(appContext)

	// --- Users (JWT) ---
	if method == "GET" && route == "/api/v1/users/me" {
		u, err := usersService.GetByID(ctx, appContext.JWTClaims.Identity)
		if err != nil {
			return ApiResponse.Error(http.StatusInternalServerError, err.Error()), nil
		}
		if u == nil {
			return ApiResponse.Error(http.StatusNotFound, "user not found"), nil
		}
		return ApiResponse.Success(u), nil
	}

	if method == "POST" && route == "/api/v1/users" {
		if appContext.JWTClaims.Role != users.RoleAdmin {
			return ApiResponse.Error(http.StatusForbidden, "admin only"), nil
		}
		var body users.CreateUserRequest
		if err := json.Unmarshal([]byte(request.Body), &body); err != nil {
			return ApiResponse.BadRequest("Invalid JSON body"), nil
		}
		data, err := usersService.CreateUser(ctx, body, commonUtils.GetCurrentTimestamp())
		if err != nil {
			return ApiResponse.Error(http.StatusBadRequest, err.Error()), nil
		}
		return ApiResponse.Success(data), nil
	}

	if method == "PATCH" && route == "/api/v1/users/password" {
		var body users.ChangePasswordRequest
		if err := json.Unmarshal([]byte(request.Body), &body); err != nil {
			return ApiResponse.BadRequest("Invalid JSON body"), nil
		}
		if err := usersService.ChangePassword(ctx, appContext.JWTClaims.Identity, appContext.JWTClaims.Role, body, commonUtils.GetCurrentTimestamp()); err != nil {
			st := http.StatusBadRequest
			if strings.Contains(err.Error(), "forbidden") {
				st = http.StatusForbidden
			}
			return ApiResponse.Error(st, err.Error()), nil
		}
		return ApiResponse.Success(map[string]string{"status": "ok"}), nil
	}

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
			var open *billing.TableOpenError
			if errors.As(err, &open) {
				return ApiResponse.Error(http.StatusConflict, err.Error()), nil
			}
			return ApiResponse.Error(http.StatusBadRequest, err.Error()), nil
		}
		for _, ord := range data.Orders {
			if pubErr := ablyUtils.PublishJSON(ctx, kitchenChannel(ord.VenueID), "order.created", ord); pubErr != nil {
				fmt.Println("ably publish error (kitchen order.created): ", pubErr)
			}
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
		if pubErr := ablyUtils.PublishJSON(ctx, kitchenChannel(data.VenueID), "order.created", data); pubErr != nil {
			fmt.Println("ably publish error (kitchen order.created): ", pubErr)
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
		if pubErr := ablyUtils.PublishJSON(ctx, kitchenChannel(data.VenueID), "order.updated", data); pubErr != nil {
			fmt.Println("ably publish error (kitchen order.updated): ", pubErr)
		}
		if data.KitchenStatus == billing.KitchenStatusReady {
			if pubErr := ablyUtils.PublishJSON(ctx, waiterChannel(data.VenueID), "order.ready", data); pubErr != nil {
				fmt.Println("ably publish error (waiter order.ready): ", pubErr)
			}
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

	if method == "POST" && route == "/api/v1/billing/invoice-number" {
		var body billing.GenerateInvoiceNumberRequest
		if err := json.Unmarshal([]byte(request.Body), &body); err != nil {
			return ApiResponse.BadRequest("Invalid JSON body"), nil
		}
		if strings.TrimSpace(body.BillID) == "" {
			return ApiResponse.BadRequest("bill_id is required"), nil
		}

		billRow, err := billRepo.GetBill(ctx, body.BillID)
		if err != nil {
			return ApiResponse.Error(http.StatusInternalServerError, err.Error()), nil
		}
		if billRow == nil {
			return ApiResponse.Error(http.StatusNotFound, "bill not found"), nil
		}

		workerResp, err := fetchInvoiceNumber(ctx, body.BillID)
		if err != nil {
			return ApiResponse.Error(http.StatusBadGateway, err.Error()), nil
		}

		billRow.InvoiceNumber = workerResp.InvoiceNumber
		billRow.UpdatedAt = commonUtils.GetCurrentTimestamp()
		if err := billRepo.PutBill(ctx, billRow); err != nil {
			return ApiResponse.Error(http.StatusInternalServerError, err.Error()), nil
		}

		return ApiResponse.Success(workerResp), nil
	}

	if method == "POST" && route == "/api/v1/loyalty/points/add" {
		var body loyalty.AddPointsRequest
		if err := json.Unmarshal([]byte(request.Body), &body); err != nil {
			return ApiResponse.BadRequest("Invalid JSON body"), nil
		}
		data, err := loyaltyService.AddPointsForBill(ctx, body, commonUtils.GetCurrentTimestamp())
		if err != nil {
			return ApiResponse.Error(http.StatusBadRequest, err.Error()), nil
		}
		return ApiResponse.Success(data), nil
	}

	if method == "GET" && route == "/api/v1/loyalty/discount" {
		userID := queryParam(request, "user_id")
		data, err := loyaltyService.GetPointsDiscount(ctx, loyalty.PointsDiscountRequest{UserID: userID})
		if err != nil {
			return ApiResponse.Error(http.StatusBadRequest, err.Error()), nil
		}
		return ApiResponse.Success(data), nil
	}

	if method == "POST" && route == "/api/v1/loyalty/discount/apply" {
		var body loyalty.ApplyDiscountRequest
		if err := json.Unmarshal([]byte(request.Body), &body); err != nil {
			return ApiResponse.BadRequest("Invalid JSON body"), nil
		}
		data, err := loyaltyService.ApplyDiscount(ctx, body, commonUtils.GetCurrentTimestamp())
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
		if pubErr := ablyUtils.PublishJSON(ctx, kitchenChannel(data.VenueID), "order.updated", data); pubErr != nil {
			fmt.Println("ably publish error (kitchen order.updated): ", pubErr)
		}
		if data.KitchenStatus == billing.KitchenStatusReady {
			if pubErr := ablyUtils.PublishJSON(ctx, waiterChannel(data.VenueID), "order.ready", data); pubErr != nil {
				fmt.Println("ably publish error (waiter order.ready): ", pubErr)
			}
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
		if pubErr := ablyUtils.PublishJSON(ctx, kitchenChannel(data.VenueID), "order.updated", data); pubErr != nil {
			fmt.Println("ably publish error (kitchen order.updated): ", pubErr)
		}
		if data.KitchenStatus == billing.KitchenStatusReady {
			if pubErr := ablyUtils.PublishJSON(ctx, waiterChannel(data.VenueID), "order.ready", data); pubErr != nil {
				fmt.Println("ably publish error (waiter order.ready): ", pubErr)
			}
		}
		return ApiResponse.Success(data), nil
	}

	return ApiResponse.Success(map[string]string{"message": "Hello, World!"}), nil
}

func main() {
	lambda.Start(handler)
}
