package menu

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// Repository loads raw sheet data from the Google Sheets API.
type Repository struct{}

// FetchSheetValues calls GET .../spreadsheets/{id}/values/{range}?key=...
// sheetName is the range (e.g. "Sheet1" or "Menu!A:Z"); it is path-escaped.
func (r *Repository) FetchSheetValues(ctx context.Context, apiKey, sheetID, sheetName string) (*SheetsValuesResponse, error) {
	encRange := url.PathEscape(sheetName)
	u := fmt.Sprintf(
		"https://sheets.googleapis.com/v4/spreadsheets/%s/values/%s?key=%s",
		sheetID,
		encRange,
		url.QueryEscape(apiKey),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google sheets API: %s", resp.Status)
	}

	var payload SheetsValuesResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return &payload, nil
}
