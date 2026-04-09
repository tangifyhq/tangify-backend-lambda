// AwsUtils provides utility functions to interact with AWS services, such as SSM Parameter Store.
// Exposed for use in other modules.
package main

import (
	"context"
	"errors"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

// ssmClient is a package-level singleton for the SSM client, private to this module.
var (
	ssmClient        *ssm.Client
	initSSMClientErr error
	ssmClientOnce    syncOnce // custom sync.Once embedding to avoid accidental export
)

// dynamodbClient is a package-level singleton for the DynamoDB client, private to this module.
var (
	dynamodbClient        *dynamodb.Client
	initDynamoDBClientErr error
	dynamodbClientOnce    syncOnce // custom sync.Once embedding to avoid accidental export
)

type syncOnce struct {
	done uint32
	m    chan struct{}
}

// Do executes the given function if it hasn't been executed yet.
func (o *syncOnce) Do(f func()) {
	if o.done == 1 {
		return
	}
	if o.m == nil {
		o.m = make(chan struct{}, 1)
	}
	select {
	case o.m <- struct{}{}:
		defer func() { o.done = 1 }()
		f()
	default:
	}
}

// AwsUtils is a struct that provides AWS-related utilities.
// No internal AWS clients; relies on package-level singleton clients.
type AwsUtils struct{}

var awsUtils *AwsUtils

// NewAwsUtils creates a new instance of AwsUtils.
func NewAwsUtils() *AwsUtils {
	if awsUtils == nil {
		awsUtils = &AwsUtils{}
		return awsUtils
	}
	return awsUtils
}

// initializeSSMClient initializes the SSM client once for this module.
func initializeSSMClient(ctx context.Context) {
	ssmClientOnce.Do(func() {
		cfg, err := config.LoadDefaultConfig(ctx)
		if err != nil {
			initSSMClientErr = err
			return
		}
		ssmClient = ssm.NewFromConfig(cfg)
	})
}

// initializeDynamoDBClient initializes the DynamoDB client once for this module.
// If DYNAMODB_ENDPOINT environment variable is set, it will use that endpoint (for local development).
// Otherwise, it will use the default AWS configuration.
func initializeDynamoDBClient() {
	dynamodbClientOnce.Do(func() {
		cfg, err := config.LoadDefaultConfig(context.TODO())
		if err != nil {
			initDynamoDBClientErr = err
			return
		}

		// Check if DYNAMODB_ENDPOINT is set (for local DynamoDB)
		endpoint := os.Getenv("DYNAMODB_ENDPOINT")
		if endpoint != "" {
			// Use custom endpoint resolver for local DynamoDB
			dynamodbClient = dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
				o.BaseEndpoint = aws.String(endpoint)
			})
		} else {
			// Use default AWS DynamoDB endpoint
			dynamodbClient = dynamodb.NewFromConfig(cfg)
		}
	})
}

// GetSSMParameter reads a parameter value from AWS Systems Manager Parameter Store.
// Assumes the environment is running in AWS Lambda or has the default SDK credentials.
func (a *AwsUtils) GetSSMParameter(ctx context.Context, parameterName string) (string, error) {
	initializeSSMClient(ctx)
	if initSSMClientErr != nil {
		return "", initSSMClientErr
	}

	paramOutput, err := ssmClient.GetParameter(context.TODO(), &ssm.GetParameterInput{
		Name:           &parameterName,
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		return "", err
	}

	if paramOutput.Parameter == nil || paramOutput.Parameter.Value == nil {
		return "", errors.New("parameter value not found")
	}

	return *paramOutput.Parameter.Value, nil
}

// GetDynamoDBClient returns the initialized DynamoDB client.
// The client is initialized on first call and reused for subsequent calls.
// For local development, set DYNAMODB_ENDPOINT environment variable (e.g., "http://localhost:8000").
func (a *AwsUtils) GetDynamoDBClient() (*dynamodb.Client, error) {
	initializeDynamoDBClient()
	if initDynamoDBClientErr != nil {
		return nil, initDynamoDBClientErr
	}
	return dynamodbClient, nil
}
