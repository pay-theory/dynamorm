# DynamORM Composite Keys Guide

## Issue Resolution

The error you're encountering is because DynamORM doesn't support the `composite:` and `extract:` syntax. The parser is interpreting comma-separated values as separate tags, which causes the "unknown tag" errors.

## Correct Approaches for Composite Keys

### Approach 1: Manual Composite Key Management

For models like `MigrationSession` where you want a single ID field containing multiple components:

```go
type MigrationSession struct {
    // Single composite ID field - manually managed
    ID        string    `dynamorm:"pk" json:"id"`
    PartnerID string    `json:"partner_id"`  // No dynamorm tag
    SessionID string    `json:"session_id"`  // No dynamorm tag
    Status    string    `json:"status"`
    CreatedAt time.Time `dynamorm:"created_at" json:"created_at"`
    UpdatedAt time.Time `dynamorm:"updated_at" json:"updated_at"`
}

// Helper methods for composite key management
func (m *MigrationSession) SetCompositeKey() {
    m.ID = fmt.Sprintf("%s#%s", m.PartnerID, m.SessionID)
}

func (m *MigrationSession) ParseCompositeKey() error {
    parts := strings.Split(m.ID, "#")
    if len(parts) != 2 {
        return fmt.Errorf("invalid composite key format: %s", m.ID)
    }
    m.PartnerID = parts[0]
    m.SessionID = parts[1]
    return nil
}

// Usage
session := &MigrationSession{
    PartnerID: "partner123",
    SessionID: "session456",
    Status:    "active",
}
session.SetCompositeKey() // Sets ID to "partner123#session456"

if err := db.Model(session).Create(); err != nil {
    return err
}
```

### Approach 2: Using PK/SK Pattern (Recommended)

For more flexible access patterns, use separate partition key and sort key fields:

```go
type MigrationSession struct {
    // Use PK/SK pattern for composite keys
    PK        string    `dynamorm:"pk" json:"pk"`        // partner_id
    SK        string    `dynamorm:"sk" json:"sk"`        // session_id
    PartnerID string    `json:"partner_id"`
    SessionID string    `json:"session_id"`
    Status    string    `json:"status"`
    CreatedAt time.Time `dynamorm:"created_at" json:"created_at"`
    UpdatedAt time.Time `dynamorm:"updated_at" json:"updated_at"`
}

// Helper to set keys
func (m *MigrationSession) SetKeys() {
    m.PK = m.PartnerID
    m.SK = m.SessionID
}

// Usage
session := &MigrationSession{
    PartnerID: "partner123",
    SessionID: "session456",
    Status:    "active",
}
session.SetKeys()

if err := db.Model(session).Create(); err != nil {
    return err
}

// Query all sessions for a partner
var sessions []MigrationSession
err := db.Model(&MigrationSession{}).
    Where("PK", "=", "partner123").
    All(&sessions)
```

### Approach 3: Generic Key Pattern for Complex Access

For models requiring multiple access patterns (like RateLimit):

```go
type RateLimit struct {
    PK           string    `dynamorm:"pk" json:"pk"`  // "PARTNER#partner_id"
    SK           string    `dynamorm:"sk" json:"sk"`  // "WINDOW#resource#timestamp"
    PartnerID    string    `json:"partner_id"`
    Resource     string    `json:"resource"`
    WindowStart  time.Time `json:"window_start"`
    RequestCount int       `json:"request_count"`
    TTL          int64     `dynamorm:"ttl" json:"ttl"`
}

func (r *RateLimit) SetKeys() {
    r.PK = fmt.Sprintf("PARTNER#%s", r.PartnerID)
    r.SK = fmt.Sprintf("WINDOW#%s#%s", r.Resource, r.WindowStart.Format(time.RFC3339))
    r.TTL = r.WindowStart.Add(time.Hour).Unix() // Expire after window
}

// Usage with atomic increment
func IncrementRateLimit(db core.DB, partnerID, resource string) error {
    windowStart := time.Now().UTC().Truncate(time.Hour)
    
    limit := &RateLimit{
        PartnerID:   partnerID,
        Resource:    resource,
        WindowStart: windowStart,
    }
    limit.SetKeys()
    
    // Atomic increment using UpdateBuilder
    return db.Model(limit).UpdateBuilder().
        Add("RequestCount", 1).
        SetIfNotExists("RequestCount", 1, 1).
        SetIfNotExists("TTL", limit.TTL, limit.TTL).
        Execute()
}
```

## Correct OAuth Token Model

```go
type OAuthToken struct {
    PK           string    `dynamorm:"pk" json:"pk"`  // partner_id
    SK           string    `dynamorm:"sk" json:"sk"`  // processor
    PartnerID    string    `json:"partner_id"`
    Processor    string    `json:"processor"`
    AccessToken  string    `json:"access_token"`
    RefreshToken string    `json:"refresh_token"`
    ExpiresAt    time.Time `json:"expires_at"`
    CreatedAt    time.Time `dynamorm:"created_at" json:"created_at"`
    UpdatedAt    time.Time `dynamorm:"updated_at" json:"updated_at"`
}

func (t *OAuthToken) SetKeys() {
    t.PK = t.PartnerID
    t.SK = t.Processor
}
```

## Global Secondary Indexes for Query Patterns

If you need to query by different attributes, define GSIs:

```go
type MigrationSession struct {
    PK        string    `dynamorm:"pk" json:"pk"`                     // partner_id
    SK        string    `dynamorm:"sk" json:"sk"`                     // session_id
    Status    string    `dynamorm:"index:status-index,pk" json:"status"`
    CreatedAt time.Time `dynamorm:"index:status-index,sk" json:"created_at"`
    // ... other fields
}

// Query by status across all partners
var sessions []MigrationSession
err := db.Model(&MigrationSession{}).
    Index("status-index").
    Where("Status", "=", "active").
    All(&sessions)
```

## Key Points

1. **No composite/extract syntax**: DynamORM doesn't support `composite:` or `extract:` tags
2. **Use PK/SK pattern**: More flexible for queries and access patterns
3. **Manual key management**: Use helper methods to construct/parse composite keys
4. **Atomic operations work**: UpdateBuilder supports atomic increments with PK/SK pattern
5. **GSIs for alternate access**: Define indexes for querying by non-key attributes

## Migration from Current Code

Replace this:
```go
// INCORRECT - Not supported
type Model struct {
    ID string `dynamorm:"pk,composite:field1,field2"`
    Field1 string `dynamorm:"extract:field1"`
    Field2 string `dynamorm:"extract:field2"`
}
```

With this:
```go
// CORRECT - PK/SK pattern
type Model struct {
    PK     string `dynamorm:"pk"`
    SK     string `dynamorm:"sk"`
    Field1 string
    Field2 string
}

func (m *Model) SetKeys() {
    m.PK = m.Field1
    m.SK = m.Field2
}
```

## Testing the Solution

```go
func TestCompositeKeyCreation(t *testing.T) {
    session := &MigrationSession{
        PartnerID: "partner123",
        SessionID: "session456",
        Status:    "active",
    }
    session.SetKeys() // PK="partner123", SK="session456"
    
    err := db.Model(session).Create()
    assert.NoError(t, err)
    
    // Query by partner
    var sessions []MigrationSession
    err = db.Model(&MigrationSession{}).
        Where("PK", "=", "partner123").
        All(&sessions)
    assert.NoError(t, err)
    assert.NotEmpty(t, sessions)
}
```

This approach should resolve all your struct tag parsing errors and provide the flexible access patterns you need. 