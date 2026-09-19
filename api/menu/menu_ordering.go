package menu

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
)

const OrderingAppSheetName = "ordering_app_menu"

// FetchOrdering downloads the ordering-app tab and maps rows by header name.
func FetchOrdering(ctx context.Context, apiKey, sheetID, sheetName string) ([]Item, error) {
	if strings.TrimSpace(sheetName) == "" {
		sheetName = OrderingAppSheetName
	}
	svc := &Service{Repo: &Repository{}}
	payload, err := svc.Repo.FetchSheetValues(ctx, apiKey, sheetID, sheetName)
	if err != nil {
		return nil, err
	}
	if len(payload.Values) == 0 {
		return []Item{}, nil
	}
	header := headerKeys(payload.Values[0])
	rows := payload.Values[1:]
	out := make([]Item, 0, len(rows))
	for _, row := range rows {
		item := mapOrderingRow(header, row)
		if strings.TrimSpace(item.Name) == "" {
			continue
		}
		out = append(out, item)
	}
	return out, nil
}

func headerKeys(row []json.RawMessage) []string {
	keys := make([]string, len(row))
	for i := range row {
		keys[i] = normalizeHeaderKey(cellString(row, i))
	}
	return keys
}

func normalizeHeaderKey(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	return strings.Join(strings.Fields(value), "_")
}

func headerIndex(header []string, key string, fallback int) int {
	for i, h := range header {
		if h == key {
			return i
		}
	}
	return fallback
}

func mapOrderingRow(header []string, row []json.RawMessage) Item {
	idx := func(key string, fallback int) int {
		return headerIndex(header, key, fallback)
	}
	status := strings.TrimSpace(cellString(row, idx("status", 0)))
	if status == "" {
		status = "OFF"
	}
	price := strings.TrimSpace(cellString(row, idx("price", 5)))
	if price == "" {
		price = "0"
	}
	return Item{
		Status:      status,
		Category:    strings.TrimSpace(cellString(row, idx("category", 1))),
		Name:        strings.TrimSpace(cellString(row, idx("name", 2))),
		Description: strings.TrimSpace(cellString(row, idx("description", 3))),
		IsVeg:       parseIsVeg(cellString(row, idx("is_veg", 4))),
		Price:       price,
	}
}

func parseIsVeg(raw string) bool {
	s := strings.ToLower(strings.TrimSpace(raw))
	switch s {
	case "veg", "true", "yes", "1":
		return true
	default:
		return false
	}
}

func cellString(row []json.RawMessage, i int) string {
	if i < 0 || i >= len(row) || len(row[i]) == 0 {
		return ""
	}
	var str string
	if err := json.Unmarshal(row[i], &str); err == nil {
		return str
	}
	var f float64
	if err := json.Unmarshal(row[i], &f); err == nil {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	var b bool
	if err := json.Unmarshal(row[i], &b); err == nil {
		if b {
			return "true"
		}
		return "false"
	}
	return strings.Trim(string(row[i]), `"`)
}
