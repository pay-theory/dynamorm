package marshal

import (
	"fmt"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/pay-theory/dynamorm/pkg/model"
)

// MarshalerType defines the type of marshaler to use
type MarshalerType string

const (
	// SafeMarshalerType uses reflection-based marshaling (RECOMMENDED)
	SafeMarshalerType MarshalerType = "safe"

	// UnsafeMarshalerType uses unsafe pointer operations (DEPRECATED)
	// Will be removed in v2.0
	UnsafeMarshalerType MarshalerType = "unsafe"
)

// MarshalerInterface defines the common interface for all marshalers
type MarshalerInterface interface {
	MarshalItem(model any, metadata *model.Metadata) (map[string]types.AttributeValue, error)
}

// Config holds marshaler configuration with security defaults
type Config struct {
	// MarshalerType specifies which marshaler to use (default: safe)
	MarshalerType MarshalerType `json:"marshaler_type" yaml:"marshaler_type"`

	// AllowUnsafeMarshaler must be explicitly set to true to enable unsafe operations
	// This flag is not serialized to prevent accidental persistence
	AllowUnsafeMarshaler bool `json:"-" yaml:"-"`

	// RequireExplicitUnsafeAck requires explicit acknowledgment of security risks
	RequireExplicitUnsafeAck bool `json:"require_explicit_unsafe_ack" yaml:"require_explicit_unsafe_ack"`

	// WarnOnUnsafeUsage logs warnings when unsafe marshaler is used
	WarnOnUnsafeUsage bool `json:"warn_on_unsafe_usage" yaml:"warn_on_unsafe_usage"`
}

// SecurityAcknowledgment represents explicit acknowledgment of unsafe marshaler risks
type SecurityAcknowledgment struct {
	AcknowledgeMemoryCorruptionRisk bool
	AcknowledgeSecurityVulnerable   bool
	AcknowledgeDeprecationWarning   bool
	DeveloperSignature              string
	Timestamp                       int64
}

var (
	// Global counters for security monitoring
	unsafeUsageCounter int64
	securityWarnings   int64
	globalConfig       Config
	configMutex        sync.RWMutex
)

// DefaultConfig returns a secure default configuration
func DefaultConfig() Config {
	return Config{
		MarshalerType:            SafeMarshalerType,
		AllowUnsafeMarshaler:     false, // Security: Default to safe
		RequireExplicitUnsafeAck: true,  // Security: Require acknowledgment
		WarnOnUnsafeUsage:        true,  // Security: Log warnings
	}
}

// SetGlobalConfig sets the global marshaler configuration
func SetGlobalConfig(config Config) {
	configMutex.Lock()
	defer configMutex.Unlock()
	globalConfig = config
}

// GetGlobalConfig returns the current global configuration
func GetGlobalConfig() Config {
	configMutex.RLock()
	defer configMutex.RUnlock()
	return globalConfig
}

// MarshalerFactory creates marshalers with security controls
type MarshalerFactory struct {
	config Config
	once   sync.Once
}

// NewMarshalerFactory creates a new factory with the given configuration
func NewMarshalerFactory(config Config) *MarshalerFactory {
	return &MarshalerFactory{config: config}
}

// NewMarshaler creates a marshaler based on configuration
func (f *MarshalerFactory) NewMarshaler() (MarshalerInterface, error) {
	return f.NewMarshalerWithAcknowledgment(nil)
}

// NewMarshalerWithAcknowledgment creates a marshaler with explicit security acknowledgment
func (f *MarshalerFactory) NewMarshalerWithAcknowledgment(ack *SecurityAcknowledgment) (MarshalerInterface, error) {
	switch f.config.MarshalerType {
	case SafeMarshalerType, "": // Default to safe
		return NewSafeMarshaler(), nil

	case UnsafeMarshalerType:
		return f.createUnsafeMarshaler(ack)

	default:
		return nil, fmt.Errorf("unknown marshaler type: %s", f.config.MarshalerType)
	}
}

// createUnsafeMarshaler creates an unsafe marshaler with security checks
func (f *MarshalerFactory) createUnsafeMarshaler(ack *SecurityAcknowledgment) (MarshalerInterface, error) {
	// Security Check 1: Must be explicitly allowed
	if !f.config.AllowUnsafeMarshaler {
		return nil, fmt.Errorf("unsafe marshaler not allowed: set AllowUnsafeMarshaler=true to enable")
	}

	// Security Check 2: Require explicit acknowledgment if configured
	if f.config.RequireExplicitUnsafeAck {
		if ack == nil {
			return nil, fmt.Errorf("unsafe marshaler requires explicit security acknowledgment")
		}

		if !ack.AcknowledgeMemoryCorruptionRisk ||
			!ack.AcknowledgeSecurityVulnerable ||
			!ack.AcknowledgeDeprecationWarning {
			return nil, fmt.Errorf("incomplete security acknowledgment for unsafe marshaler")
		}

		if ack.DeveloperSignature == "" {
			return nil, fmt.Errorf("developer signature required for unsafe marshaler acknowledgment")
		}
	}

	// Security Warning: Log usage
	if f.config.WarnOnUnsafeUsage {
		atomic.AddInt64(&unsafeUsageCounter, 1)
		atomic.AddInt64(&securityWarnings, 1)

		log.Printf("⚠️  SECURITY WARNING: Using deprecated unsafe marshaler")
		log.Printf("   - Memory corruption risk: CRITICAL")
		log.Printf("   - Security vulnerability: HIGH")
		log.Printf("   - Deprecated: Will be removed in v2.0")
		log.Printf("   - Usage count: %d", atomic.LoadInt64(&unsafeUsageCounter))
		log.Printf("   - Consider migrating to safe marshaler")

		if ack != nil {
			log.Printf("   - Acknowledged by: %s", ack.DeveloperSignature)
		}
	}

	// Check environment variable override (for CI/testing)
	if os.Getenv("DYNAMORM_FORCE_SAFE_MARSHALER") == "true" {
		log.Printf("🔒 SECURITY OVERRIDE: Forcing safe marshaler (DYNAMORM_FORCE_SAFE_MARSHALER=true)")
		return NewSafeMarshaler(), nil
	}

	// Create the unsafe marshaler (from existing code)
	return New(), nil // This will use the existing unsafe implementation
}

// GetSecurityStats returns security-related statistics
func GetSecurityStats() SecurityStats {
	return SecurityStats{
		UnsafeUsageCount: atomic.LoadInt64(&unsafeUsageCounter),
		SecurityWarnings: atomic.LoadInt64(&securityWarnings),
		CurrentConfig:    GetGlobalConfig(),
	}
}

// SecurityStats contains security monitoring information
type SecurityStats struct {
	UnsafeUsageCount int64
	SecurityWarnings int64
	CurrentConfig    Config
}

// CreateSecurityAcknowledgment creates a security acknowledgment for unsafe marshaler usage
func CreateSecurityAcknowledgment(developerSignature string) *SecurityAcknowledgment {
	return &SecurityAcknowledgment{
		AcknowledgeMemoryCorruptionRisk: true,
		AcknowledgeSecurityVulnerable:   true,
		AcknowledgeDeprecationWarning:   true,
		DeveloperSignature:              developerSignature,
		Timestamp:                       time.Now().Unix(),
	}
}

// ValidateConfig validates marshaler configuration for security compliance
func ValidateConfig(config Config) error {
	if config.MarshalerType == UnsafeMarshalerType {
		if !config.AllowUnsafeMarshaler {
			return fmt.Errorf("unsafe marshaler type specified but not explicitly allowed")
		}

		if config.RequireExplicitUnsafeAck && !config.WarnOnUnsafeUsage {
			log.Printf("⚠️  WARNING: Unsafe marshaler acknowledgment required but warnings disabled")
		}
	}

	return nil
}

// init initializes the global configuration with secure defaults
func init() {
	globalConfig = DefaultConfig()

	// Check for environment variable overrides
	if marshalerType := os.Getenv("DYNAMORM_MARSHALER_TYPE"); marshalerType != "" {
		switch marshalerType {
		case "safe":
			globalConfig.MarshalerType = SafeMarshalerType
		case "unsafe":
			globalConfig.MarshalerType = UnsafeMarshalerType
			globalConfig.AllowUnsafeMarshaler = true
			log.Printf("⚠️  SECURITY WARNING: Unsafe marshaler enabled via environment variable")
		}
	}
}
