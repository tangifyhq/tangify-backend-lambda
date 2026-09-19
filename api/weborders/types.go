package weborders

import "tangify-backend-lambda/menu"

// PublicMenuItem is the JSON shape written to R2 for order.tangify.in.
// Matches tangify-ordering-web MenuItem (no internal_name / sop).
type PublicMenuItem struct {
	Status      string `json:"status"`
	Category    string `json:"category"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IsVeg       bool   `json:"is_veg"`
	Price       string `json:"price"`
}

// PublishResult is returned to POS after a successful menu sync.
type PublishResult struct {
	PublishedAt string `json:"published_at"`
	ItemCount   int    `json:"item_count"`
	CategoryCount int  `json:"category_count"`
	PublicURL   string `json:"public_url"`
	ObjectKey   string `json:"object_key"`
}

// Config holds env-backed settings for menu publish.
type Config struct {
	SheetsAPIKey string
	SheetID      string
	SheetName    string

	R2AccountID       string
	R2AccessKeyID     string
	R2SecretAccessKey string
	R2Bucket          string
	R2MenuKey         string

	PublicBaseURL string

	// Optional Cloudflare cache purge
	CFAPIToken string
	CFZoneID   string
}

// ToPublicItems maps sheet items to the public CDN payload.
// Only status ON items are included (case-insensitive).
func ToPublicItems(items []menu.Item) []PublicMenuItem {
	out := make([]PublicMenuItem, 0, len(items))
	for _, item := range items {
		status := item.Status
		if status == "" {
			status = "OFF"
		}
		out = append(out, PublicMenuItem{
			Status:      status,
			Category:    item.Category,
			Name:        item.Name,
			Description: item.Description,
			IsVeg:       item.IsVeg,
			Price:       item.Price,
		})
	}
	return out
}

func countCategories(items []PublicMenuItem) int {
	seen := map[string]struct{}{}
	for _, item := range items {
		cat := item.Category
		if cat == "" {
			cat = "Menu"
		}
		seen[cat] = struct{}{}
	}
	return len(seen)
}
