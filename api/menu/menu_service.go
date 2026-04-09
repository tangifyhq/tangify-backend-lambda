package menu

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
)

// Service maps Google Sheet rows to menu items (same columns as houseofodia-menu route.ts).
type Service struct {
	Repo *Repository
}

// Fetch downloads the sheet and returns parsed menu items.
func (s *Service) Fetch(ctx context.Context, apiKey, sheetID, sheetName string) ([]Item, error) {
	payload, err := s.Repo.FetchSheetValues(ctx, apiKey, sheetID, sheetName)
	if err != nil {
		return nil, err
	}
	if len(payload.Values) == 0 {
		return []Item{}, nil
	}

	_, rows := payload.Values[0], payload.Values[1:]
	out := make([]Item, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapRow(row))
	}
	return out, nil
}

// Fetch is a package-level helper using a default Service.
func Fetch(ctx context.Context, apiKey, sheetID, sheetName string) ([]Item, error) {
	svc := &Service{Repo: &Repository{}}
	return svc.Fetch(ctx, apiKey, sheetID, sheetName)
}

func mapRow(row []json.RawMessage) Item {
	cell := func(i int) string {
		if i >= len(row) || len(row[i]) == 0 {
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

	status := cell(0)
	if status == "" {
		status = "OFF"
	}
	isVeg := strings.ToLower(strings.TrimSpace(cell(4))) == "veg"
	price := cell(5)
	if price == "" {
		price = "0"
	}

	return Item{
		Status:      status,
		Category:    strings.TrimSpace(cell(1)),
		Name:        cell(2),
		Description: cell(3),
		IsVeg:       isVeg,
		Price:       price,
	}
}
