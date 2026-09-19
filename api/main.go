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
	"tangify-backend-lambda/reviews"
	"tangify-backend-lambda/users"
	"tangify-backend-lambda/weborders"
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
	"/api/v1/loyalty/otp/send",
	"/api/v1/loyalty/otp/verify",
	"/api/v1/menu",
	"/api/v1/health",
	"/api/v1/reviews/generate",
	"/api/v1/webhooks/gupshup",
	"/api/v1/web/auth/continue",
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
	token := strings.TrimPrefix(headerGet(request.Headers, "authorization"), "Bearer ")
	token = strings.TrimSpace(token)
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

	if method == "POST" && route == "/api/v1/reviews/generate" {
		var body reviews.GenerateReviewRequest
		if err := json.Unmarshal([]byte(request.Body), &body); err != nil {
			return ApiResponse.BadRequest("Invalid JSON body"), nil
		}

		menuItemNames := []string{}
		apiKey := os.Getenv("GOOGLE_SHEETS_API_KEY")
		sheetID := os.Getenv("GOOGLE_SHEET_ID")
		sheetName := os.Getenv("GOOGLE_SHEET_NAME")
		if apiKey != "" && sheetID != "" {
			items, menuErr := menu.Fetch(ctx, apiKey, sheetID, sheetName)
			if menuErr != nil {
				fmt.Println("review generate menu fetch error: ", menuErr)
			} else {
				menuItemNames = menu.ShuffleStrings(
					menu.ActiveItemNamesInCategories(items, menu.ReviewContextCategories),
				)
			}
		}

		reviewService := reviews.NewService(os.Getenv("LLM_API_KEY"))
		data, err := reviewService.Generate(ctx, body, menuItemNames)
		if err != nil {
			fmt.Println("review generate error: ", err)
			return ApiResponse.Error(reviews.ErrorStatus(err), err.Error()), nil
		}
		return ApiResponse.Success(data), nil
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

	billingEnvironment := "production"
	billsWithLineItemsTable := billing.TableNameBillsWithLineItems
	pointsWalletTable := loyalty.TableNamePointsWallet
	if strings.EqualFold(
		strings.TrimSpace(request.Headers["x-tangify-environment"]),
		"dev",
	) {
		billingEnvironment = "dev"
		billsWithLineItemsTable = billing.DevTableNameBillsWithLineItems
		pointsWalletTable = loyalty.DevTableNamePointsWallet
	}

	usersService := users.NewService(users.NewRepository(dynamoDBClient), func(userID, name, role string) (string, error) {
		j := NewJwtUtils(jwtSecret)
		ttl := 24 * time.Hour
		if role == users.RoleCustomer {
			// Web ordering sessions should last across a shopping trip / next-day return.
			ttl = 30 * 24 * time.Hour
		}
		return j.GenerateJWT(userID, name, role, ttl)
	})
	billRepo := billing.NewRepository(dynamoDBClient)
	loyaltyRepo := loyalty.NewRepository(dynamoDBClient, pointsWalletTable)
	loyaltyService := loyalty.NewService(loyaltyRepo, billRepo)
	walletProvider := loyalty.NewWalletProvider(loyaltyRepo, usersService)

	if method == "POST" && route == "/api/v1/webhooks/gupshup" {
		loginSvc := weborders.NewLoginService(
			weborders.NewLoginNonceRepository(dynamoDBClient),
			usersService,
		)
		if err := handleGupshupInbound(
			ctx,
			request.Body,
			walletProvider,
			loginSvc,
			commonUtils.GetCurrentTimestamp(),
		); err != nil {
			fmt.Println("gupshup webhook error:", err)
			return ApiResponse.Error(http.StatusInternalServerError, err.Error()), nil
		}
		return ApiResponse.Success(map[string]string{"status": "ok"}), nil
	}

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

	otpService := loyalty.NewOTPService(
		loyalty.NewOTPRepository(dynamoDBClient),
		loyaltyRepo,
		usersService,
		jwtSecret,
		sendGupshupOTPMessage,
	)

	if method == "POST" && route == "/api/v1/loyalty/otp/send" {
		var body loyalty.SendOTPRequest
		if err := json.Unmarshal([]byte(request.Body), &body); err != nil {
			return ApiResponse.BadRequest("Invalid JSON body"), nil
		}
		data, err := otpService.Send(ctx, body, commonUtils.GetCurrentTimestamp())
		if err != nil {
			return ApiResponse.Error(loyalty.OTPErrorStatus(err), err.Error()), nil
		}
		return ApiResponse.Success(data), nil
	}

	if method == "POST" && route == "/api/v1/loyalty/otp/verify" {
		var body loyalty.VerifyOTPRequest
		if err := json.Unmarshal([]byte(request.Body), &body); err != nil {
			return ApiResponse.BadRequest("Invalid JSON body"), nil
		}
		data, err := otpService.Verify(ctx, body, commonUtils.GetCurrentTimestamp())
		if err != nil {
			return ApiResponse.Error(loyalty.OTPErrorStatus(err), err.Error()), nil
		}
		return ApiResponse.Success(data), nil
	}

	if method == "POST" && route == "/api/v1/web/auth/continue" {
		var body weborders.ContinueRequest
		if err := json.Unmarshal([]byte(request.Body), &body); err != nil {
			return ApiResponse.BadRequest("Invalid JSON body"), nil
		}
		code := body.Code()
		if code == "" {
			return ApiResponse.BadRequest("key required"), nil
		}
		loginSvc := weborders.NewLoginService(
			weborders.NewLoginNonceRepository(dynamoDBClient),
			usersService,
		)
		data, err := loginSvc.ConsumeNonce(ctx, code, commonUtils.GetCurrentTimestamp())
		if err != nil {
			fmt.Println("web login continue error: ", err)
			return ApiResponse.Error(http.StatusInternalServerError, err.Error()), nil
		}
		if data == nil {
			return ApiResponse.Error(http.StatusNotFound, "login link expired or already used"), nil
		}
		return ApiResponse.Success(data), nil
	}

	if method == "GET" && strings.HasPrefix(route, "/api/v1/web/orders/") {
		ref := strings.TrimPrefix(route, "/api/v1/web/orders/")
		ref = strings.Trim(ref, "/")
		if ref == "" || strings.Contains(ref, "/") {
			return ApiResponse.BadRequest("order_ref required"), nil
		}
		webOrderSvc := weborders.NewWebOrderService(
			weborders.NewWebOrderRepository(dynamoDBClient),
			weborders.NewRazorpayService(),
		)
		order, err := webOrderSvc.GetByRef(ctx, ref)
		if err != nil {
			fmt.Println("web order get error: ", err)
			return ApiResponse.Error(http.StatusInternalServerError, err.Error()), nil
		}
		if order == nil {
			return ApiResponse.Error(http.StatusNotFound, "order not found"), nil
		}
		return ApiResponse.Success(order), nil
	}

	if !slices.Contains(whitelistedRoutes, route) {
		err = doJwtAuth(request, jwtSecret, appContext)
		if err != nil {
			return ApiResponse.Unauthorized(fmt.Sprintf("Unauthorized: %v", err)), nil
		}
	}

	billingService := billing.NewService(billRepo)
	invoiceWorkerURL := billing.ResolveInvoiceWorkerURL(billingEnvironment)
	billsWithLineItemsRepo := billing.NewBillWithLineItemsRepository(
		dynamoDBClient,
		billsWithLineItemsTable,
	)
	billsWithLineItemsService := billing.NewBillWithLineItemsService(
		billsWithLineItemsRepo,
		walletProvider,
		invoiceWorkerURL,
		gupshupLoyaltyNotifier{},
		pointsWalletTable,
	)
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
			if pubErr := ablyPublisher().PublishJSON(ctx, kitchenChannel(ord.VenueID), "order.created", ord); pubErr != nil {
				fmt.Println("ably publish error (kitchen order.created): ", pubErr)
			}
		}
		return ApiResponse.Success(data), nil
	}

	if method == "PATCH" && route == "/api/v1/billing/sessions" {
		var body billing.UpdateSessionRequest
		if err := json.Unmarshal([]byte(request.Body), &body); err != nil {
			return ApiResponse.BadRequest("Invalid JSON body"), nil
		}
		data, err := billingService.UpdateSession(ctx, body, commonUtils)
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
		if pubErr := ablyPublisher().PublishJSON(ctx, kitchenChannel(data.VenueID), "order.created", data); pubErr != nil {
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
		if pubErr := ablyPublisher().PublishJSON(ctx, kitchenChannel(data.VenueID), "order.updated", data); pubErr != nil {
			fmt.Println("ably publish error (kitchen order.updated): ", pubErr)
		}
		if data.KitchenStatus == billing.KitchenStatusReady {
			if pubErr := ablyPublisher().PublishJSON(ctx, waiterChannel(data.VenueID), "order.ready", data); pubErr != nil {
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

		workerResp, err := fetchInvoiceNumber(ctx, body.BillID, invoiceWorkerURL)
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

	if method == "PUT" && route == "/api/v1/billing/bills/with-line-items" {
		var body billing.UpsertBillWithLineItemsRequest
		if err := json.Unmarshal([]byte(request.Body), &body); err != nil {
			return ApiResponse.BadRequest("Invalid JSON body"), nil
		}
		data, err := billsWithLineItemsService.Upsert(ctx, body, staffID, commonUtils.GetCurrentTimestamp())
		if err != nil {
			st := http.StatusBadRequest
			if errors.Is(err, billing.ErrBillNotFound) {
				st = http.StatusNotFound
			}
			return ApiResponse.Error(st, err.Error()), nil
		}
		return ApiResponse.Success(data), nil
	}

	if method == "GET" && route == "/api/v1/billing/bills/with-line-items" {
		billID := queryParam(request, "bill_id")
		data, err := billsWithLineItemsService.Get(ctx, billID)
		if err != nil {
			return ApiResponse.Error(http.StatusBadRequest, err.Error()), nil
		}
		if data == nil {
			return ApiResponse.Error(http.StatusNotFound, "bill not found"), nil
		}
		return ApiResponse.Success(data), nil
	}

	if method == "GET" && route == "/api/v1/loyalty/wallet" {
		phone := queryParam(request, "phone")
		data, err := walletProvider.GetOrCreateByPhone(ctx, phone, commonUtils.GetCurrentTimestamp())
		if err != nil {
			return ApiResponse.Error(http.StatusBadRequest, err.Error()), nil
		}
		return ApiResponse.Success(data), nil
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
		if pubErr := ablyPublisher().PublishJSON(ctx, kitchenChannel(data.VenueID), "order.updated", data); pubErr != nil {
			fmt.Println("ably publish error (kitchen order.updated): ", pubErr)
		}
		if data.KitchenStatus == billing.KitchenStatusReady {
			if pubErr := ablyPublisher().PublishJSON(ctx, waiterChannel(data.VenueID), "order.ready", data); pubErr != nil {
				fmt.Println("ably publish error (waiter order.ready): ", pubErr)
			}
		}
		return ApiResponse.Success(data), nil
	}

	if method == "GET" && route == "/api/v1/kitchen/unit-board" {
		data, err := billingService.KitchenUnitBoard(ctx, queryParam(request, "venue_id"))
		if err != nil {
			return ApiResponse.Error(http.StatusInternalServerError, err.Error()), nil
		}
		return ApiResponse.Success(data), nil
	}

	if method == "PATCH" && route == "/api/v1/kitchen/line-items/unit" {
		var body billing.PatchLineItemUnitRequest
		if err := json.Unmarshal([]byte(request.Body), &body); err != nil {
			return ApiResponse.BadRequest("Invalid JSON body"), nil
		}
		data, err := billingService.PatchLineItemUnit(ctx, body, commonUtils)
		if err != nil {
			return ApiResponse.Error(http.StatusBadRequest, err.Error()), nil
		}
		if pubErr := ablyPublisher().PublishJSON(ctx, kitchenChannel(data.VenueID), "order.updated", data); pubErr != nil {
			fmt.Println("ably publish error (kitchen order.updated): ", pubErr)
		}
		if data.KitchenStatus == billing.KitchenStatusReady {
			if pubErr := ablyPublisher().PublishJSON(ctx, waiterChannel(data.VenueID), "order.ready", data); pubErr != nil {
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
		if pubErr := ablyPublisher().PublishJSON(ctx, kitchenChannel(data.VenueID), "order.updated", data); pubErr != nil {
			fmt.Println("ably publish error (kitchen order.updated): ", pubErr)
		}
		if data.KitchenStatus == billing.KitchenStatusReady {
			if pubErr := ablyPublisher().PublishJSON(ctx, waiterChannel(data.VenueID), "order.ready", data); pubErr != nil {
				fmt.Println("ably publish error (waiter order.ready): ", pubErr)
			}
		}
		return ApiResponse.Success(data), nil
	}

	// --- Web ordering ---
	if method == "POST" && route == "/api/v1/web/menu/publish" {
		if appContext.JWTClaims == nil || !weborders.CanPublishRole(appContext.JWTClaims.Role) {
			return ApiResponse.Error(http.StatusForbidden, "staff only"), nil
		}
		webSvc := weborders.NewService()
		data, err := webSvc.PublishMenu(ctx, weborders.ConfigFromEnv())
		if err != nil {
			fmt.Println("web menu publish error: ", err)
			return ApiResponse.Error(http.StatusInternalServerError, err.Error()), nil
		}
		return ApiResponse.Success(data), nil
	}

	if method == "GET" && route == "/api/v1/web/loyalty/wallet" {
		if appContext.JWTClaims == nil {
			return ApiResponse.Unauthorized("Unauthorized"), nil
		}
		data, err := walletProvider.GetWebWallet(ctx, appContext.JWTClaims.Identity)
		if err != nil {
			fmt.Println("web loyalty wallet error: ", err)
			return ApiResponse.Error(http.StatusBadRequest, err.Error()), nil
		}
		return ApiResponse.Success(data), nil
	}

	if method == "POST" && route == "/api/v1/web/payments/razorpay/order" {
		if appContext.JWTClaims == nil {
			return ApiResponse.Unauthorized("Unauthorized"), nil
		}
		var body weborders.CreateRazorpayOrderRequest
		if err := json.Unmarshal([]byte(request.Body), &body); err != nil {
			return ApiResponse.BadRequest("Invalid JSON body"), nil
		}
		if body.Notes == nil {
			body.Notes = map[string]string{}
		}
		body.Notes["user_id"] = appContext.JWTClaims.Identity
		data, err := weborders.NewRazorpayService().CreateOrder(
			ctx,
			weborders.RazorpayConfigFromEnv(),
			body,
		)
		if err != nil {
			fmt.Println("razorpay create order error: ", err)
			st := http.StatusBadRequest
			if strings.Contains(err.Error(), "not configured") {
				st = http.StatusInternalServerError
			}
			return ApiResponse.Error(st, err.Error()), nil
		}
		return ApiResponse.Success(data), nil
	}

	if method == "POST" && route == "/api/v1/web/delivery/quote" {
		if appContext.JWTClaims == nil {
			return ApiResponse.Unauthorized("Unauthorized"), nil
		}
		var body weborders.DeliveryQuoteRequest
		if err := json.Unmarshal([]byte(request.Body), &body); err != nil {
			return ApiResponse.BadRequest("Invalid JSON body"), nil
		}
		data, err := weborders.NewShiprocketService().QuoteDelivery(
			ctx,
			weborders.ShiprocketConfigFromEnv(),
			body,
		)
		if err != nil {
			fmt.Println("delivery quote error: ", err)
			msg := err.Error()
			st := http.StatusBadRequest
			switch {
			case strings.Contains(msg, "not configured"):
				st = http.StatusInternalServerError
				msg = "Delivery quotes are temporarily unavailable"
			case strings.Contains(msg, "delivery_postcode"),
				strings.Contains(msg, "latitude and longitude"):
				// validation — keep message
			default:
				st = http.StatusBadGateway
				msg = "Could not get delivery fee — try again"
			}
			return ApiResponse.Error(st, msg), nil
		}
		return ApiResponse.Success(data), nil
	}

	if method == "POST" && route == "/api/v1/web/payments/razorpay/verify" {
		if appContext.JWTClaims == nil {
			return ApiResponse.Unauthorized("Unauthorized"), nil
		}
		var body weborders.VerifyRazorpayPaymentRequest
		if err := json.Unmarshal([]byte(request.Body), &body); err != nil {
			return ApiResponse.BadRequest("Invalid JSON body"), nil
		}
		webOrderSvc := weborders.NewWebOrderService(
			weborders.NewWebOrderRepository(dynamoDBClient),
			weborders.NewRazorpayService(),
		)
		data, err := webOrderSvc.VerifyAndSave(
			ctx,
			weborders.RazorpayConfigFromEnv(),
			appContext.JWTClaims.Identity,
			body,
			commonUtils.GetCurrentTimestamp(),
		)
		if err != nil {
			fmt.Println("razorpay verify error: ", err)
			return ApiResponse.Error(http.StatusBadRequest, err.Error()), nil
		}
		return ApiResponse.Success(data), nil
	}

	if method == "GET" && route == "/api/v1/web/orders" {
		if appContext.JWTClaims == nil {
			return ApiResponse.Unauthorized("Unauthorized"), nil
		}
		webOrderSvc := weborders.NewWebOrderService(
			weborders.NewWebOrderRepository(dynamoDBClient),
			weborders.NewRazorpayService(),
		)
		orders, err := webOrderSvc.ListForUser(ctx, appContext.JWTClaims.Identity)
		if err != nil {
			fmt.Println("web orders list error: ", err)
			return ApiResponse.Error(http.StatusInternalServerError, err.Error()), nil
		}
		return ApiResponse.Success(map[string]any{"orders": orders}), nil
	}

	addrSvc := weborders.NewWebAddressService(weborders.NewWebAddressRepository(dynamoDBClient))

	if method == "GET" && route == "/api/v1/web/addresses" {
		if appContext.JWTClaims == nil {
			return ApiResponse.Unauthorized("Unauthorized"), nil
		}
		list, err := addrSvc.List(ctx, appContext.JWTClaims.Identity)
		if err != nil {
			fmt.Println("web addresses list error: ", err)
			return ApiResponse.Error(http.StatusInternalServerError, err.Error()), nil
		}
		return ApiResponse.Success(map[string]any{"addresses": list}), nil
	}

	if method == "PUT" && route == "/api/v1/web/addresses" {
		if appContext.JWTClaims == nil {
			return ApiResponse.Unauthorized("Unauthorized"), nil
		}
		var body weborders.UpsertWebAddressRequest
		if err := json.Unmarshal([]byte(request.Body), &body); err != nil {
			return ApiResponse.BadRequest("Invalid JSON body"), nil
		}
		addr, err := addrSvc.Upsert(ctx, appContext.JWTClaims.Identity, body)
		if err != nil {
			fmt.Println("web address upsert error: ", err)
			return ApiResponse.Error(http.StatusBadRequest, err.Error()), nil
		}
		return ApiResponse.Success(addr), nil
	}

	if method == "DELETE" && strings.HasPrefix(route, "/api/v1/web/addresses/") {
		if appContext.JWTClaims == nil {
			return ApiResponse.Unauthorized("Unauthorized"), nil
		}
		id := strings.TrimPrefix(route, "/api/v1/web/addresses/")
		id = strings.Trim(id, "/")
		if id == "" || strings.Contains(id, "/") {
			return ApiResponse.BadRequest("address_id required"), nil
		}
		if err := addrSvc.Delete(ctx, appContext.JWTClaims.Identity, id); err != nil {
			fmt.Println("web address delete error: ", err)
			st := http.StatusBadRequest
			if strings.Contains(err.Error(), "not found") {
				st = http.StatusNotFound
			}
			return ApiResponse.Error(st, err.Error()), nil
		}
		return ApiResponse.Success(map[string]bool{"deleted": true}), nil
	}

	return ApiResponse.Success(map[string]string{"message": "Hello, World!"}), nil
}

func main() {
	lambda.Start(handler)
}
