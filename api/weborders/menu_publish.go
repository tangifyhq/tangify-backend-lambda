package weborders

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"tangify-backend-lambda/menu"
)

// Service publishes the Google Sheet menu to R2 for the web ordering PWA.
type Service struct{}

func NewService() *Service {
	return &Service{}
}

// PublishMenu fetches the sheet, writes menu/v1.json (+ current pointer), and optionally purges CF cache.
func (s *Service) PublishMenu(ctx context.Context, cfg Config) (*PublishResult, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	items, err := menu.FetchOrdering(ctx, cfg.SheetsAPIKey, cfg.SheetID, cfg.SheetName)
	if err != nil {
		return nil, fmt.Errorf("fetch sheet: %w", err)
	}

	public := ToPublicItems(items)
	body, err := json.Marshal(public)
	if err != nil {
		return nil, fmt.Errorf("marshal menu: %w", err)
	}

	if err := ensurePublicReadCORS(ctx, cfg); err != nil {
		// Non-fatal: object upload can still succeed; browsers may fail without CORS.
		fmt.Println("web menu CORS ensure warning:", err)
	}

	publishedAt := time.Now().In(time.FixedZone("IST", 5*60*60+30*60)).Format(time.RFC3339)

	// Short TTL so price/status changes propagate without requiring a new filename.
	const menuCacheControl = "public, max-age=60, stale-while-revalidate=300"
	if err := putJSONObject(ctx, cfg, cfg.R2MenuKey, body, menuCacheControl); err != nil {
		return nil, fmt.Errorf("upload menu: %w", err)
	}

	pointer := map[string]any{
		"url":          cfg.R2MenuKey,
		"published_at": publishedAt,
		"item_count":   len(public),
	}
	pointerBody, err := json.Marshal(pointer)
	if err != nil {
		return nil, fmt.Errorf("marshal pointer: %w", err)
	}
	pointerKey := "menu/current.json"
	if err := putJSONObject(ctx, cfg, pointerKey, pointerBody, "public, max-age=0, must-revalidate"); err != nil {
		return nil, fmt.Errorf("upload menu pointer: %w", err)
	}

	publicURL := cfg.PublicBaseURL + "/" + cfg.R2MenuKey
	_ = purgeCloudflareURLs(ctx, cfg, []string{
		publicURL,
		cfg.PublicBaseURL + "/" + pointerKey,
	})

	return &PublishResult{
		PublishedAt:   publishedAt,
		ItemCount:     len(public),
		CategoryCount: countCategories(public),
		PublicURL:     publicURL,
		ObjectKey:     cfg.R2MenuKey,
	}, nil
}

func purgeCloudflareURLs(ctx context.Context, cfg Config, urls []string) error {
	if cfg.CFAPIToken == "" || cfg.CFZoneID == "" {
		return nil
	}
	payload, err := json.Marshal(map[string]any{"files": urls})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/purge_cache", cfg.CFZoneID),
		bytes.NewReader(payload),
	)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.CFAPIToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("cloudflare purge: %s", resp.Status)
	}
	return nil
}

// CanPublishRole returns true for staff / service tokens allowed to sync the web menu.
// Customer and kitchen JWTs are blocked; empty role is allowed (POS service token).
func CanPublishRole(role string) bool {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "customer", "kitchen":
		return false
	default:
		return true
	}
}
