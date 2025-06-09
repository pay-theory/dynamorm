// multiaccount.go
package dynamorm

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/pay-theory/dynamorm/pkg/session"
)

// MultiAccountDB manages DynamoDB connections across multiple AWS accounts
type MultiAccountDB struct {
	baseDB        *LambdaDB
	accounts      map[string]AccountConfig
	cache         *sync.Map // Cache DB connections per account
	baseConfig    aws.Config
	refreshTicker *time.Ticker
	refreshStop   chan struct{}
	mu            sync.RWMutex
}

// AccountConfig holds configuration for a partner account
type AccountConfig struct {
	RoleARN    string
	ExternalID string
	Region     string
	// Optional: Custom session duration (default is 1 hour)
	SessionDuration time.Duration
}

// NewMultiAccount creates a multi-account aware DB
func NewMultiAccount(accounts map[string]AccountConfig) (*MultiAccountDB, error) {
	baseDB, err := NewLambdaOptimized()
	if err != nil {
		return nil, fmt.Errorf("failed to create base Lambda DB: %w", err)
	}

	// Load base AWS config
	baseConfig, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to load base AWS config: %w", err)
	}

	mdb := &MultiAccountDB{
		baseDB:      baseDB,
		accounts:    accounts,
		cache:       &sync.Map{},
		baseConfig:  baseConfig,
		refreshStop: make(chan struct{}),
	}

	// Start credential refresh routine
	mdb.startCredentialRefresh()

	return mdb, nil
}

// Partner returns a DB instance for the specified partner account
func (mdb *MultiAccountDB) Partner(partnerID string) (*LambdaDB, error) {
	// Empty partner ID returns base DB
	if partnerID == "" {
		return mdb.baseDB, nil
	}

	// Check cache first
	if cached, ok := mdb.cache.Load(partnerID); ok {
		if entry, ok := cached.(*cacheEntry); ok && !entry.isExpired() {
			return entry.db, nil
		}
	}

	// Get account config
	mdb.mu.RLock()
	account, ok := mdb.accounts[partnerID]
	mdb.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("unknown partner: %s", partnerID)
	}

	// Create or refresh DB for partner
	return mdb.createPartnerDB(partnerID, account)
}

// AddPartner dynamically adds a new partner configuration
func (mdb *MultiAccountDB) AddPartner(partnerID string, config AccountConfig) {
	mdb.mu.Lock()
	defer mdb.mu.Unlock()
	mdb.accounts[partnerID] = config
}

// RemovePartner removes a partner and clears its cached connection
func (mdb *MultiAccountDB) RemovePartner(partnerID string) {
	mdb.mu.Lock()
	delete(mdb.accounts, partnerID)
	mdb.mu.Unlock()

	mdb.cache.Delete(partnerID)
}

// createPartnerDB creates a new DB instance for a partner account
func (mdb *MultiAccountDB) createPartnerDB(partnerID string, account AccountConfig) (*LambdaDB, error) {
	// Create STS client
	stsClient := sts.NewFromConfig(mdb.baseConfig)

	// Set session duration (default to 1 hour)
	sessionDuration := account.SessionDuration
	if sessionDuration == 0 {
		sessionDuration = time.Hour
	}

	// Create credentials provider for assume role
	creds := stscreds.NewAssumeRoleProvider(stsClient, account.RoleARN, func(o *stscreds.AssumeRoleOptions) {
		o.ExternalID = &account.ExternalID
		o.RoleSessionName = fmt.Sprintf("dynamorm-%s", partnerID)
		o.Duration = sessionDuration
	})

	// Create new config with assumed role
	awsConfigOptions := []func(*config.LoadOptions) error{
		config.WithRegion(account.Region),
		config.WithCredentialsProvider(creds),
	}

	// Add Lambda optimizations if in Lambda environment
	if IsLambdaEnvironment() {
		httpClient := &http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
				DisableKeepAlives:   false,
			},
		}
		awsConfigOptions = append(awsConfigOptions,
			config.WithHTTPClient(httpClient),
			config.WithRetryMode(aws.RetryModeAdaptive),
			config.WithRetryMaxAttempts(3),
		)
	}

	// Create partner-specific session config
	cfg := session.Config{
		Region:           account.Region,
		MaxRetries:       3,
		DefaultRCU:       5,
		DefaultWCU:       5,
		AutoMigrate:      false,
		EnableMetrics:    IsLambdaEnvironment(),
		AWSConfigOptions: awsConfigOptions,
	}

	// Create partner DB
	db, err := New(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create partner DB for %s: %w", partnerID, err)
	}

	lambdaDB := &LambdaDB{
		DB:             db,
		modelCache:     &sync.Map{},
		isLambda:       IsLambdaEnvironment(),
		lambdaMemoryMB: GetLambdaMemoryMB(),
		xrayEnabled:    EnableXRayTracing(),
	}

	// Cache with expiration
	entry := &cacheEntry{
		db:         lambdaDB,
		expiry:     time.Now().Add(sessionDuration - 5*time.Minute), // Refresh 5 minutes before expiry
		partnerID:  partnerID,
		accountCfg: account,
	}
	mdb.cache.Store(partnerID, entry)

	return lambdaDB, nil
}

// startCredentialRefresh starts a background routine to refresh credentials
func (mdb *MultiAccountDB) startCredentialRefresh() {
	mdb.refreshTicker = time.NewTicker(5 * time.Minute)

	go func() {
		for {
			select {
			case <-mdb.refreshTicker.C:
				mdb.refreshExpiredCredentials()
			case <-mdb.refreshStop:
				return
			}
		}
	}()
}

// refreshExpiredCredentials checks and refreshes expired credentials
func (mdb *MultiAccountDB) refreshExpiredCredentials() {
	now := time.Now()

	mdb.cache.Range(func(key, value any) bool {
		partnerID := key.(string)
		entry := value.(*cacheEntry)

		// Check if credentials are about to expire
		if now.After(entry.expiry.Add(-10 * time.Minute)) {
			// Refresh in background
			go func() {
				_, err := mdb.createPartnerDB(partnerID, entry.accountCfg)
				if err != nil {
					// Log error but don't fail - next request will retry
					fmt.Printf("Failed to refresh credentials for partner %s: %v\n", partnerID, err)
				}
			}()
		}

		return true
	})
}

// Close stops the refresh routine and cleans up
func (mdb *MultiAccountDB) Close() error {
	if mdb.refreshTicker != nil {
		mdb.refreshTicker.Stop()
	}
	close(mdb.refreshStop)
	return mdb.baseDB.Close()
}

// WithContext returns a new MultiAccountDB with the given context
func (mdb *MultiAccountDB) WithContext(ctx context.Context) *MultiAccountDB {
	// Create new MultiAccountDB without copying sync.Map
	newMDB := &MultiAccountDB{
		baseDB:        mdb.baseDB.WithLambdaTimeout(ctx),
		accounts:      mdb.accounts,
		baseConfig:    mdb.baseConfig,
		refreshTicker: mdb.refreshTicker,
		refreshStop:   mdb.refreshStop,
	}
	// Share the same cache pointer
	newMDB.cache = mdb.cache
	return newMDB
}

// cacheEntry holds a cached DB connection with expiration
type cacheEntry struct {
	db         *LambdaDB
	expiry     time.Time
	partnerID  string
	accountCfg AccountConfig
}

// isExpired checks if the cache entry has expired
func (e *cacheEntry) isExpired() bool {
	return time.Now().After(e.expiry)
}

// PartnerContext adds partner information to context for tracing
func PartnerContext(ctx context.Context, partnerID string) context.Context {
	return context.WithValue(ctx, partnerContextKey{}, partnerID)
}

// GetPartnerFromContext retrieves partner ID from context
func GetPartnerFromContext(ctx context.Context) string {
	if partnerID, ok := ctx.Value(partnerContextKey{}).(string); ok {
		return partnerID
	}
	return ""
}

type partnerContextKey struct{}
