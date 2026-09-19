package weborders

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	"tangify-backend-lambda/menu"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func envTrim(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}

func ConfigFromEnv() Config {
	menuKey := envTrim("R2_MENU_KEY")
	if menuKey == "" {
		menuKey = "menu/v1.json"
	}
	publicBase := envTrim("WEB_MENU_PUBLIC_BASE_URL")
	if publicBase == "" {
		publicBase = "https://files.tangify.in"
	}
	sheetName := envTrim("GOOGLE_ORDERING_SHEET_NAME")
	if sheetName == "" {
		sheetName = menu.OrderingAppSheetName
	}
	return Config{
		SheetsAPIKey:      envTrim("GOOGLE_SHEETS_API_KEY"),
		SheetID:           envTrim("GOOGLE_SHEET_ID"),
		SheetName:         sheetName,
		R2AccountID:       envTrim("R2_ACCOUNT_ID"),
		R2AccessKeyID:     envTrim("R2_ACCESS_KEY_ID"),
		R2SecretAccessKey: envTrim("R2_SECRET_ACCESS_KEY"),
		R2Bucket:          envTrim("R2_BUCKET"),
		R2MenuKey:         strings.Trim(menuKey, "/"),
		PublicBaseURL:     strings.TrimRight(publicBase, "/"),
		CFAPIToken:        envTrim("CF_API_TOKEN"),
		CFZoneID:          envTrim("CF_ZONE_ID"),
	}
}

func (c Config) Validate() error {
	if c.SheetsAPIKey == "" || c.SheetID == "" {
		return fmt.Errorf("Google Sheets is not configured")
	}
	if c.R2AccountID == "" || c.R2AccessKeyID == "" || c.R2SecretAccessKey == "" || c.R2Bucket == "" {
		return fmt.Errorf("R2 is not configured (R2_ACCOUNT_ID, R2_ACCESS_KEY_ID, R2_SECRET_ACCESS_KEY, R2_BUCKET)")
	}
	return nil
}

func r2Endpoint(accountID string) string {
	return fmt.Sprintf("https://%s.r2.cloudflarestorage.com", accountID)
}

func newR2Client(cfg Config) *s3.Client {
	awsCfg := aws.Config{
		Region: "auto",
		Credentials: credentials.NewStaticCredentialsProvider(
			cfg.R2AccessKeyID,
			cfg.R2SecretAccessKey,
			"",
		),
		BaseEndpoint: aws.String(r2Endpoint(cfg.R2AccountID)),
	}
	return s3.NewFromConfig(awsCfg)
}

func putJSONObject(ctx context.Context, cfg Config, key string, body []byte, cacheControl string) error {
	client := newR2Client(cfg)
	_, err := client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:       aws.String(cfg.R2Bucket),
		Key:          aws.String(key),
		Body:         bytes.NewReader(body),
		ContentType:  aws.String("application/json"),
		CacheControl: aws.String(cacheControl),
	})
	return err
}

// ensurePublicReadCORS allows browsers on order.tangify.in / localhost to fetch menu JSON.
func ensurePublicReadCORS(ctx context.Context, cfg Config) error {
	client := newR2Client(cfg)
	_, err := client.PutBucketCors(ctx, &s3.PutBucketCorsInput{
		Bucket: aws.String(cfg.R2Bucket),
		CORSConfiguration: &s3types.CORSConfiguration{
			CORSRules: []s3types.CORSRule{
				{
					AllowedHeaders: []string{"*"},
					AllowedMethods: []string{http.MethodGet, http.MethodHead},
					AllowedOrigins: []string{"*"},
					ExposeHeaders:  []string{"ETag", "Content-Type", "Cache-Control"},
					MaxAgeSeconds:  aws.Int32(3600),
				},
			},
		},
	})
	return err
}
