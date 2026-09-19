package weborders

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const defaultWebOrderNumberWorkerURL = "https://tangify-web-order-number-generator.subnub.workers.dev/"
const webOrderNumberWorkerURLEnvProd = "WEB_ORDER_NUMBER_WORKER_URL_PROD"
const webOrderNumberWorkerURLEnvDev = "WEB_ORDER_NUMBER_WORKER_URL_DEV"

type webOrderNumberWorkerRequest struct {
	AttemptID string `json:"attempt_id"`
}

// WebOrderNumberResponse is returned by the web order number CF worker.
type WebOrderNumberResponse struct {
	OrderRef  string `json:"order_ref"`
	AttemptID string `json:"attempt_id"`
	Year      int    `json:"year"`
	Sequence  int    `json:"sequence"`
}

func ResolveWebOrderNumberWorkerURL(environment string) string {
	if strings.EqualFold(strings.TrimSpace(environment), "dev") {
		if v := strings.TrimSpace(os.Getenv(webOrderNumberWorkerURLEnvDev)); v != "" {
			return v
		}
	}
	if v := strings.TrimSpace(os.Getenv(webOrderNumberWorkerURLEnvProd)); v != "" {
		return v
	}
	return defaultWebOrderNumberWorkerURL
}

// FetchWebOrderNumber allocates a sequential TGFY-W-YYYY-NNNNNN ref from the worker.
func FetchWebOrderNumber(ctx context.Context, attemptID string) (*WebOrderNumberResponse, error) {
	return FetchWebOrderNumberWithURL(
		ctx,
		attemptID,
		ResolveWebOrderNumberWorkerURL("production"),
	)
}

func FetchWebOrderNumberWithURL(
	ctx context.Context,
	attemptID string,
	workerURL string,
) (*WebOrderNumberResponse, error) {
	attemptID = strings.TrimSpace(attemptID)
	if attemptID == "" {
		return nil, fmt.Errorf("attempt_id required")
	}
	targetURL := strings.TrimSpace(workerURL)
	if targetURL == "" {
		targetURL = defaultWebOrderNumberWorkerURL
	}

	b, err := json.Marshal(webOrderNumberWorkerRequest{AttemptID: attemptID})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		targetURL,
		bytes.NewReader(b),
	)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("web order number worker status=%d body=%s", resp.StatusCode, string(respBody))
	}

	out := &WebOrderNumberResponse{}
	if err := json.Unmarshal(respBody, out); err != nil {
		return nil, err
	}
	if strings.TrimSpace(out.OrderRef) == "" {
		return nil, fmt.Errorf("web order number worker returned empty order_ref")
	}
	out.OrderRef = NormalizeOrderRef(out.OrderRef)
	return out, nil
}
