package weborders

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const shiprocketAPIBase = "https://apiv2.shiprocket.in/v1/external"

type ShiprocketConfig struct {
	Email          string
	Password       string
	PickupPostcode string
	PickupLat      float64
	PickupLng      float64
	DefaultWeight  float64
}

func ShiprocketConfigFromEnv() ShiprocketConfig {
	lat, _ := strconv.ParseFloat(strings.TrimSpace(os.Getenv("SHIPROCKET_PICKUP_LAT")), 64)
	lng, _ := strconv.ParseFloat(strings.TrimSpace(os.Getenv("SHIPROCKET_PICKUP_LNG")), 64)
	if lat == 0 && lng == 0 {
		lat = 12.8616
		lng = 77.786
	}
	weight, _ := strconv.ParseFloat(strings.TrimSpace(os.Getenv("SHIPROCKET_DEFAULT_WEIGHT_KG")), 64)
	if weight <= 0 {
		weight = 0.5
	}
	pin := strings.TrimSpace(os.Getenv("SHIPROCKET_PICKUP_POSTCODE"))
	if pin == "" {
		pin = "562125"
	}
	return ShiprocketConfig{
		Email:          strings.TrimSpace(os.Getenv("SHIPROCKET_EMAIL")),
		Password:       strings.TrimSpace(os.Getenv("SHIPROCKET_PASSWORD")),
		PickupPostcode: pin,
		PickupLat:      lat,
		PickupLng:      lng,
		DefaultWeight:  weight,
	}
}

func (c ShiprocketConfig) Validate() error {
	if c.Email == "" || c.Password == "" {
		return fmt.Errorf("shiprocket is not configured")
	}
	if c.PickupPostcode == "" {
		return fmt.Errorf("shiprocket pickup postcode is not configured")
	}
	return nil
}

type DeliveryQuoteRequest struct {
	DeliveryPostcode string  `json:"delivery_postcode"`
	Latitude         float64 `json:"latitude"`
	Longitude        float64 `json:"longitude"`
	WeightKg         float64 `json:"weight_kg,omitempty"`
}

type DeliveryQuoteResponse struct {
	Serviceable    bool    `json:"serviceable"`
	FeePaise       int64   `json:"fee_paise"`
	CourierName    string  `json:"courier_name,omitempty"`
	CourierID      int64   `json:"courier_id,omitempty"`
	ETD            string  `json:"etd,omitempty"`
	EtdHours       float64 `json:"etd_hours,omitempty"`
	DistanceKm     float64 `json:"distance_km,omitempty"`
	PickupPostcode string  `json:"pickup_postcode"`
}

type shiprocketCourier struct {
	ID            int64   `json:"courier_company_id"`
	Name          string  `json:"courier_name"`
	Rate          float64 `json:"rate"`
	Rates         float64 `json:"rates"` // hyperlocal list responses use "rates"
	ETD           string  `json:"etd"`
	EtdHours      float64 `json:"etd_hours"`
	FreightCharge float64 `json:"freight_charge"`
	DistanceKm    float64 `json:"distance"`
}

type ShiprocketService struct {
	httpClient *http.Client
	apiBase    string

	mu           sync.Mutex
	cachedToken  string
	tokenExpires time.Time
}

func NewShiprocketService() *ShiprocketService {
	return &ShiprocketService{
		httpClient: &http.Client{Timeout: 20 * time.Second},
		apiBase:    shiprocketAPIBase,
	}
}

func (s *ShiprocketService) QuoteDelivery(
	ctx context.Context,
	cfg ShiprocketConfig,
	req DeliveryQuoteRequest,
) (*DeliveryQuoteResponse, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	pin := digitsOnly(req.DeliveryPostcode)
	if len(pin) != 6 {
		return nil, fmt.Errorf("delivery_postcode must be a 6-digit Indian pincode")
	}
	if req.Latitude == 0 || req.Longitude == 0 {
		return nil, fmt.Errorf("latitude and longitude are required")
	}
	weight := req.WeightKg
	if weight <= 0 {
		weight = cfg.DefaultWeight
	}

	authToken, err := s.getToken(ctx, cfg)
	if err != nil {
		return nil, err
	}

	q := url.Values{}
	q.Set("pickup_postcode", cfg.PickupPostcode)
	q.Set("delivery_postcode", pin)
	q.Set("cod", "0")
	q.Set("weight", strconv.FormatFloat(weight, 'f', 2, 64))
	q.Set("is_new_hyperlocal", "1")
	q.Set("lat_from", strconv.FormatFloat(cfg.PickupLat, 'f', 6, 64))
	q.Set("long_from", strconv.FormatFloat(cfg.PickupLng, 'f', 6, 64))
	q.Set("lat_to", strconv.FormatFloat(req.Latitude, 'f', 6, 64))
	q.Set("long_to", strconv.FormatFloat(req.Longitude, 'f', 6, 64))

	couriers, err := s.serviceability(ctx, authToken, q)
	if err != nil {
		// Retry once on auth failure with fresh token.
		if strings.Contains(err.Error(), "401") || strings.Contains(strings.ToLower(err.Error()), "token") {
			s.invalidateToken()
			authToken, err2 := s.getToken(ctx, cfg)
			if err2 != nil {
				return nil, err2
			}
			couriers, err = s.serviceability(ctx, authToken, q)
		}
		if err != nil {
			return nil, err
		}
	}

	best, ok := pickBestCourier(couriers)
	if !ok {
		return &DeliveryQuoteResponse{
			Serviceable:    false,
			PickupPostcode: cfg.PickupPostcode,
		}, nil
	}

	feeINR := courierFee(best)
	if feeINR <= 0 {
		return &DeliveryQuoteResponse{
			Serviceable:    false,
			PickupPostcode: cfg.PickupPostcode,
		}, nil
	}
	feePaise := int64(feeINR*100 + 0.5)

	distanceKm := best.DistanceKm
	if distanceKm <= 0 {
		distanceKm = haversineKm(cfg.PickupLat, cfg.PickupLng, req.Latitude, req.Longitude)
	}

	return &DeliveryQuoteResponse{
		Serviceable:    true,
		FeePaise:       feePaise,
		CourierName:    best.Name,
		CourierID:      best.ID,
		ETD:            best.ETD,
		EtdHours:       best.EtdHours,
		DistanceKm:     distanceKm,
		PickupPostcode: cfg.PickupPostcode,
	}, nil
}

func (s *ShiprocketService) getToken(ctx context.Context, cfg ShiprocketConfig) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cachedToken != "" && time.Now().Before(s.tokenExpires) {
		return s.cachedToken, nil
	}
	body, _ := json.Marshal(map[string]string{
		"email":    cfg.Email,
		"password": cfg.Password,
	})
	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		strings.TrimRight(s.apiBase, "/")+"/auth/login",
		bytes.NewReader(body),
	)
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("shiprocket login failed: %s", strings.TrimSpace(string(respBody)))
	}
	var parsed struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil || parsed.Token == "" {
		return "", fmt.Errorf("shiprocket login: empty token")
	}
	s.cachedToken = parsed.Token
	s.tokenExpires = time.Now().Add(9 * 24 * time.Hour)
	return s.cachedToken, nil
}

func (s *ShiprocketService) invalidateToken() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cachedToken = ""
	s.tokenExpires = time.Time{}
}

func (s *ShiprocketService) serviceability(
	ctx context.Context,
	token string,
	q url.Values,
) ([]shiprocketCourier, error) {
	u := strings.TrimRight(s.apiBase, "/") + "/courier/serviceability/?" + q.Encode()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("shiprocket serviceability %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	list, err := parseServiceabilityBody(respBody)
	if err != nil {
		return nil, err
	}
	return list, nil
}

// parseServiceabilityBody accepts Shiprocket shapes where data is an object
// with courier lists, or data is [] / null when nothing is serviceable.
func parseServiceabilityBody(body []byte) ([]shiprocketCourier, error) {
	var top struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &top); err != nil {
		return nil, fmt.Errorf("shiprocket serviceability: invalid response")
	}
	data := bytes.TrimSpace(top.Data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) || bytes.Equal(data, []byte("[]")) {
		return nil, nil
	}
	if data[0] == '[' {
		// Rare: data is a courier array directly.
		list := decodeCourierSlice(data)
		if len(list) == 0 {
			list = parseCourierAnySlice(data)
		}
		return list, nil
	}
	if data[0] != '{' {
		return nil, nil
	}

	var obj struct {
		AvailableCourierCompanies json.RawMessage `json:"available_courier_companies"`
		AvailableCourierCompany   json.RawMessage `json:"available_courier_company"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil, nil
	}
	list := decodeCourierSlice(obj.AvailableCourierCompanies)
	if len(list) == 0 {
		list = decodeCourierSlice(obj.AvailableCourierCompany)
	}
	if len(list) == 0 {
		list = parseCouriersLoose(body)
	}
	return list, nil
}

func decodeCourierSlice(raw json.RawMessage) []shiprocketCourier {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) || bytes.Equal(raw, []byte("[]")) {
		return nil
	}
	var list []shiprocketCourier
	if err := json.Unmarshal(raw, &list); err != nil || len(list) == 0 {
		return parseCourierAnySlice(raw)
	}
	// Rates sometimes arrive as strings — refill zeros via loose parse.
	needLoose := false
	for _, c := range list {
		if c.Name != "" && courierFee(c) <= 0 {
			needLoose = true
			break
		}
	}
	if needLoose {
		loose := parseCourierAnySlice(raw)
		if len(loose) > 0 {
			return loose
		}
	}
	return list
}

func parseCouriersLoose(body []byte) []shiprocketCourier {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil
	}
	data, ok := raw["data"].(map[string]any)
	if !ok || data == nil {
		return nil
	}
	arr, _ := data["available_courier_companies"].([]any)
	if len(arr) == 0 {
		arr, _ = data["available_courier_company"].([]any)
	}
	return couriersFromAny(arr)
}

func parseCourierAnySlice(raw json.RawMessage) []shiprocketCourier {
	var arr []any
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil
	}
	return couriersFromAny(arr)
}

func couriersFromAny(arr []any) []shiprocketCourier {
	out := make([]shiprocketCourier, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		c := shiprocketCourier{
			Name:          strings.TrimSpace(fmt.Sprint(m["courier_name"])),
			ETD:           strings.TrimSpace(fmt.Sprint(m["etd"])),
			EtdHours:      toFloat(m["etd_hours"]),
			Rate:          toFloat(m["rate"]),
			Rates:         toFloat(m["rates"]),
			FreightCharge: toFloat(m["freight_charge"]),
			ID:            int64(toFloat(m["courier_company_id"])),
			DistanceKm:    toFloat(m["distance"]),
		}
		if c.Name == "" || c.Name == "<nil>" {
			continue
		}
		out = append(out, c)
	}
	return out
}

func toFloat(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(t), 64)
		return f
	case json.Number:
		f, _ := t.Float64()
		return f
	default:
		return 0
	}
}

func pickBestCourier(couriers []shiprocketCourier) (shiprocketCourier, bool) {
	if len(couriers) == 0 {
		return shiprocketCourier{}, false
	}
	pool := make([]shiprocketCourier, 0, len(couriers))
	for _, c := range couriers {
		if courierFee(c) <= 0 {
			continue
		}
		if isQuickCourier(c.Name) {
			pool = append(pool, c)
		}
	}
	if len(pool) == 0 {
		for _, c := range couriers {
			if courierFee(c) > 0 {
				pool = append(pool, c)
			}
		}
	}
	if len(pool) == 0 {
		return shiprocketCourier{}, false
	}
	best := pool[0]
	bestRate := courierFee(best)
	for _, c := range pool[1:] {
		r := courierFee(c)
		if r < bestRate {
			best = c
			bestRate = r
		}
	}
	return best, true
}

func courierFee(c shiprocketCourier) float64 {
	if c.Rate > 0 {
		return c.Rate
	}
	if c.Rates > 0 {
		return c.Rates
	}
	return c.FreightCharge
}

func isQuickCourier(name string) bool {
	n := strings.ToLower(name)
	deny := []string{"delhivery", "blue dart", "bluedart", "xpressbees", "ecom"}
	for _, d := range deny {
		if strings.Contains(n, d) {
			return false
		}
	}
	allow := []string{"quick", "flash", "rapido", "loadshare", "ola", "shadowfax hyperlocal"}
	for _, a := range allow {
		if strings.Contains(n, a) {
			return true
		}
	}
	return false
}

func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func haversineKm(lat1, lng1, lat2, lng2 float64) float64 {
	const earthKm = 6371.0
	toRad := func(d float64) float64 { return d * math.Pi / 180 }
	dLat := toRad(lat2 - lat1)
	dLng := toRad(lng2 - lng1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(toRad(lat1))*math.Cos(toRad(lat2))*math.Sin(dLng/2)*math.Sin(dLng/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	km := earthKm * c
	if km < 0.1 {
		return 0.1
	}
	return math.Round(km*10) / 10
}
