package menu

import "encoding/json"

// Item is one menu row after the header row.
type Item struct {
	Status      string `json:"status"`
	Category    string `json:"category"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IsVeg       bool   `json:"is_veg"`
	Price       string `json:"price"`
}

// SheetsValuesResponse is the JSON shape returned by Google Sheets API v4 values.get.
type SheetsValuesResponse struct {
	Values [][]json.RawMessage `json:"values"`
}
