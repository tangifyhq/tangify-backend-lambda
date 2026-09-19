package main

import (
	"context"
	"testing"

	"github.com/aws/aws-lambda-go/events"
)

func TestHandlerHealth(t *testing.T) {
	t.Setenv("ABLY_KEY", "abc.def:test-secret")
	resetAblyPublisherForTest()

	resp, err := handler(context.Background(), events.LambdaFunctionURLRequest{
		RequestContext: events.LambdaFunctionURLRequestContext{
			HTTP: events.LambdaFunctionURLRequestContextHTTPDescription{Method: "GET"},
		},
		RawPath: "/api/v1/health",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if ablyShared != nil {
		t.Fatal("GET /health must not initialize Ably")
	}
}

func TestHandlerMenuDoesNotCreateAblyClient(t *testing.T) {
	t.Setenv("ABLY_KEY", "abc.def:test-secret")
	t.Setenv("GOOGLE_SHEETS_API_KEY", "")
	t.Setenv("GOOGLE_SHEET_ID", "")
	resetAblyPublisherForTest()

	_, err := handler(context.Background(), events.LambdaFunctionURLRequest{
		RequestContext: events.LambdaFunctionURLRequestContext{
			HTTP: events.LambdaFunctionURLRequestContextHTTPDescription{Method: "GET"},
		},
		RawPath: "/api/v1/menu",
	})
	if err != nil {
		t.Fatal(err)
	}
	if ablyShared != nil {
		t.Fatal("GET /menu must not initialize Ably")
	}
}

