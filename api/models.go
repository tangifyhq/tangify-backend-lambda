package main

import "errors"

var (
	DefaultHTTPGetAddress = "https://checkip.amazonaws.com"

	ErrNoIP = errors.New("No IP in HTTP response")

	ErrNon200Response = errors.New("Non 200 Response found")

	ErrMissingJWT           = errors.New("missing JWT")
	ErrInvalidJWT           = errors.New("invalid JWT")
	ErrFailedToGetJWTSecret = errors.New("failed to get JWT secret")
)

type ProductType struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	PriceInPaise int64  `json:"price_in_paise"`
	Description  string `json:"description"`
}
