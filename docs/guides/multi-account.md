# Multi-Account Setup Guide

DynamORM provides built-in support for working with DynamoDB tables across multiple AWS accounts. This guide shows you how to configure and use multi-account access.

## Table of Contents

- [Overview](#overview)
- [Prerequisites](#prerequisites)
- [Configuration](#configuration)
- [Usage Examples](#usage-examples)
- [Security Best Practices](#security-best-practices)
- [Common Patterns](#common-patterns)
- [Troubleshooting](#troubleshooting)

## Overview

Multi-account access is useful for:
- **Separation of Environments**: Dev, staging, and production in different accounts
- **Multi-Tenant Applications**: Each tenant in a separate account
- **Cross-Team Access**: Accessing data from other teams' accounts
- **Data Migration**: Moving data between accounts
- **Backup and Disaster Recovery**: Cross-account backups

## Prerequisites

### 1. IAM Roles Setup

In the target account(s), create an IAM role that can be assumed:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "AWS": "arn:aws:iam::SOURCE_ACCOUNT_ID:root"
      },
      "Action": "sts:AssumeRole",
      "Condition": {
        "StringEquals": {
          "sts:ExternalId": "unique-external-id"
        }
      }
    }
  ]
}
```

Attach a policy for DynamoDB access:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "dynamodb:*"
      ],
      "Resource": [
        "arn:aws:dynamodb:REGION:ACCOUNT_ID:table/TABLE_NAME",
        "arn:aws:dynamodb:REGION:ACCOUNT_ID:table/TABLE_NAME/index/*"
      ]
    }
  ]
}
```

### 2. Source Account Permissions

Ensure your source account (or Lambda role) can assume the target roles:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": "sts:AssumeRole",
      "Resource": [
        "arn:aws:iam::TARGET_ACCOUNT_1:role/DynamoDBAccessRole",
        "arn:aws:iam::TARGET_ACCOUNT_2:role/DynamoDBAccessRole"
      ]
    }
  ]
}
```

## Configuration

### Basic Multi-Account Setup

```go
import "github.com/dynamorm/dynamorm"

// Configure multi-account access
db, err := dynamorm.New(
    dynamorm.WithMultiAccount(map[string]string{
        "dev":  "arn:aws:iam::111111111111:role/DynamoDBAccessRole",
        "prod": "arn:aws:iam::222222222222:role/DynamoDBAccessRole",
        "backup": "arn:aws:iam::333333333333:role/BackupRole",
    }),
    dynamorm.WithRegion("us-east-1"),
)
```

### With External ID (Recommended)

```go
db, err := dynamorm.New(
    dynamorm.WithMultiAccount(map[string]string{
        "dev":  "arn:aws:iam::111111111111:role/DynamoDBAccessRole",
        "prod": "arn:aws:iam::222222222222:role/DynamoDBAccessRole",
    }),
    dynamorm.WithExternalID("unique-external-id-12345"),
)
```

### With Custom Session Duration

```go
db, err := dynamorm.New(
    dynamorm.WithMultiAccount(map[string]string{
        "prod": "arn:aws:iam::222222222222:role/DynamoDBAccessRole",
    }),
    dynamorm.WithSessionDuration(time.Hour * 2), // Up to 12 hours
)
```

## Usage Examples

### Switching Between Accounts

```go
// Query from dev account
var devUsers []User
err := db.WithAccount("dev").
    Model(&User{}).
    All(&devUsers)

// Query from prod account
var prodUsers []User
err = db.WithAccount("prod").
    Model(&User{}).
    All(&prodUsers)
```

### Cross-Account Data Copy

```go
// Read from production
var users []User
err := db.WithAccount("prod").
    Model(&User{}).
    Where("Status", "=", "active").
    All(&users)

// Write to backup account
err = db.WithAccount("backup").
    Model(&User{}).
    BatchCreate(users)
```

### Account-Specific Models

```go
// Define models with account information
type User struct {
    ID    string `dynamorm:"pk"`
    Email string
    Name  string
}

func (User) TableName() string {
    return "users"
}

func (User) Account() string {
    return "prod" // Always use prod account for User model
}

// Usage - automatically uses prod account
err := db.Model(&User{}).Create(&user)
```

### Dynamic Account Selection

```go
func GetUsersByTenant(tenantID string) ([]User, error) {
    // Map tenants to accounts
    accountMap := map[string]string{
        "tenant-a": "account-a",
        "tenant-b": "account-b",
        "tenant-c": "account-c",
    }
    
    account, ok := accountMap[tenantID]
    if !ok {
        return nil, errors.New("unknown tenant")
    }
    
    var users []User
    err := db.WithAccount(account).
        Model(&User{}).
        All(&users)
    
    return users, err
}
```

## Security Best Practices

### 1. Use External IDs

Always use external IDs to prevent confused deputy attacks:

```go
db, err := dynamorm.New(
    dynamorm.WithMultiAccount(accounts),
    dynamorm.WithExternalID(os.Getenv("MULTI_ACCOUNT_EXTERNAL_ID")),
)
```

### 2. Principle of Least Privilege

Grant minimal permissions needed:

```json
{
  "Effect": "Allow",
  "Action": [
    "dynamodb:GetItem",
    "dynamodb:Query",
    "dynamodb:BatchGetItem"
  ],
  "Resource": "arn:aws:dynamodb:*:*:table/users"
}
```

### 3. Audit Logging

Enable CloudTrail for cross-account access:

```go
// Add metadata to operations for audit trail
err := db.WithAccount("prod").
    WithContext(ctx).
    WithMetadata(map[string]string{
        "operation": "data-migration",
        "requestor": userID,
        "reason": "backup",
    }).
    Model(&User{}).
    All(&users)
```

### 4. Credential Rotation

Implement automatic credential rotation:

```go
// Refresh credentials periodically
ticker := time.NewTicker(30 * time.Minute)
go func() {
    for range ticker.C {
        db.RefreshCredentials()
    }
}()
```

## Common Patterns

### Multi-Environment Pattern

```go
type DBConfig struct {
    db  *dynamorm.DB
    env string
}

func NewMultiEnvDB() (*DBConfig, error) {
    db, err := dynamorm.New(
        dynamorm.WithMultiAccount(map[string]string{
            "dev":     "arn:aws:iam::111111:role/DynamoDBRole",
            "staging": "arn:aws:iam::222222:role/DynamoDBRole",
            "prod":    "arn:aws:iam::333333:role/DynamoDBRole",
        }),
    )
    
    if err != nil {
        return nil, err
    }
    
    return &DBConfig{
        db:  db,
        env: os.Getenv("ENVIRONMENT"),
    }, nil
}

func (c *DBConfig) Query(model interface{}) *dynamorm.Query {
    return c.db.WithAccount(c.env).Model(model)
}
```

### Tenant Isolation Pattern

```go
type TenantDB struct {
    db       *dynamorm.DB
    tenantID string
}

func NewTenantDB(tenantID string) (*TenantDB, error) {
    accounts := loadTenantAccountMapping()
    
    db, err := dynamorm.New(
        dynamorm.WithMultiAccount(accounts),
    )
    
    if err != nil {
        return nil, err
    }
    
    return &TenantDB{
        db:       db,
        tenantID: tenantID,
    }, nil
}

func (t *TenantDB) Model(model interface{}) *dynamorm.Query {
    account := getTenantAccount(t.tenantID)
    return t.db.WithAccount(account).Model(model)
}
```

### Cross-Account Replication

```go
func ReplicateTable(sourceAccount, targetAccount, tableName string) error {
    // Create paginated reader
    cursor := ""
    
    for {
        // Read batch from source
        result, err := db.WithAccount(sourceAccount).
            Model(&GenericItem{}).
            Table(tableName).
            Cursor(cursor).
            Limit(100).
            Paginate()
        
        if err != nil {
            return err
        }
        
        // Write batch to target
        err = db.WithAccount(targetAccount).
            Model(&GenericItem{}).
            Table(tableName).
            BatchCreate(result.Items)
        
        if err != nil {
            return err
        }
        
        if !result.HasMore() {
            break
        }
        
        cursor = result.NextCursor
    }
    
    return nil
}
```

## Troubleshooting

### Common Issues

#### 1. AssumeRole Access Denied

**Error**: `AccessDenied: User is not authorized to perform: sts:AssumeRole`

**Solution**:
- Check IAM role trust relationship
- Verify external ID matches
- Ensure source has AssumeRole permission

#### 2. Credential Expiration

**Error**: `ExpiredToken: The security token included in the request is expired`

**Solution**:
```go
// Enable automatic credential refresh
db, err := dynamorm.New(
    dynamorm.WithMultiAccount(accounts),
    dynamorm.WithAutoRefresh(true),
)
```

#### 3. Wrong Region

**Error**: `ResourceNotFoundException: Requested resource not found`

**Solution**:
```go
// Specify region per account
db.WithAccount("prod").WithRegion("eu-west-1").Model(&User{})
```

### Debug Mode

Enable debug logging for troubleshooting:

```go
db, err := dynamorm.New(
    dynamorm.WithMultiAccount(accounts),
    dynamorm.WithDebug(true),
)

// Logs will show:
// - Account switches
// - Role assumptions
// - Credential refreshes
```

### Health Checks

Implement health checks for multi-account access:

```go
func CheckMultiAccountAccess(db *dynamorm.DB, accounts []string) error {
    for _, account := range accounts {
        // Try to describe a table in each account
        err := db.WithAccount(account).
            Model(&HealthCheck{}).
            Table("health-check").
            Exists()
        
        if err != nil {
            return fmt.Errorf("account %s health check failed: %w", account, err)
        }
    }
    return nil
}
```

## Best Practices Summary

1. **Always use External IDs** for cross-account roles
2. **Implement least privilege** access policies
3. **Monitor cross-account access** with CloudTrail
4. **Handle credential expiration** gracefully
5. **Test failover scenarios** between accounts
6. **Document account mappings** clearly
7. **Use environment variables** for configuration

## Next Steps

- Set up IAM roles in your target accounts
- Configure multi-account access in your application
- Implement monitoring and alerting
- Test cross-account operations
- Review [Security Best Practices](security.md)

---

<p align="center">
  🌍 Multi-account access made simple with DynamORM!
</p> 