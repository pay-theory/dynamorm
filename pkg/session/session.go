// Package session provides AWS session management and DynamoDB client configuration
package session

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

// Config holds the configuration for DynamORM
type Config struct {
	// AWS region
	Region string

	// Optional endpoint for local development (e.g., DynamoDB Local)
	Endpoint string

	// Maximum number of retries for failed requests
	MaxRetries int

	// Default read capacity units for table creation
	DefaultRCU int64

	// Default write capacity units for table creation
	DefaultWCU int64

	// Whether to automatically create tables if they don't exist
	AutoMigrate bool

	// Whether to enable metrics collection
	EnableMetrics bool

	// Custom AWS config options
	AWSConfigOptions []func(*config.LoadOptions) error

	// Custom DynamoDB client options
	DynamoDBOptions []func(*dynamodb.Options)
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		Region:        "us-east-1",
		MaxRetries:    3,
		DefaultRCU:    5,
		DefaultWCU:    5,
		AutoMigrate:   false,
		EnableMetrics: false,
	}
}

// Session manages the AWS session and DynamoDB client
type Session struct {
	config    *Config
	awsConfig aws.Config
	client    *dynamodb.Client
}

// NewSession creates a new session with the given configuration
func NewSession(cfg *Config) (*Session, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	// Build AWS config options
	options := make([]func(*config.LoadOptions) error, 0, len(cfg.AWSConfigOptions)+2)

	// Add region if specified
	if cfg.Region != "" {
		options = append(options, config.WithRegion(cfg.Region))
	}

	// Add retry configuration
	if cfg.MaxRetries > 0 {
		options = append(options, config.WithRetryMaxAttempts(cfg.MaxRetries))
	}

	// Add custom options
	options = append(options, cfg.AWSConfigOptions...)

	// Load AWS config
	awsConfig, err := config.LoadDefaultConfig(context.Background(), options...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create DynamoDB client
	clientOptions := &dynamodb.Options{
		Region: awsConfig.Region,
	}

	// Apply endpoint override if specified
	if cfg.Endpoint != "" {
		clientOptions.BaseEndpoint = aws.String(cfg.Endpoint)
	}

	// Create client with options
	client := dynamodb.NewFromConfig(awsConfig, func(o *dynamodb.Options) {
		*o = *clientOptions
		// Apply custom DynamoDB options
		for _, opt := range cfg.DynamoDBOptions {
			opt(o)
		}
	})

	return &Session{
		config:    cfg,
		awsConfig: awsConfig,
		client:    client,
	}, nil
}

// Client returns the DynamoDB client
func (s *Session) Client() *dynamodb.Client {
	return s.client
}

// Config returns the session configuration
func (s *Session) Config() *Config {
	return s.config
}

// AWSConfig returns the AWS configuration
func (s *Session) AWSConfig() aws.Config {
	return s.awsConfig
}

// WithContext returns a new session with the given context
func (s *Session) WithContext(ctx context.Context) *Session {
	// DynamoDB client operations accept context at the operation level
	// This method is here for consistency with the DB interface
	return s
}
