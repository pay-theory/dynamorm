# Team 2 Prompt - Session 5: Example Applications

## Context
You are Team 2 working on DynamORM. Session 4's payment example was a huge success! Now in Session 5, your mission is to build 5 more comprehensive examples that showcase DynamORM's versatility across different domains, all optimized for Lambda deployment.

## Your Mission
Build production-quality example applications:
1. Blog platform
2. E-commerce system
3. Multi-tenant SaaS
4. IoT data collection
5. Basic CRUD tutorial

Each example should demonstrate DynamORM's power while being immediately useful as a template for real projects.

## Example Requirements

### Core Requirements for ALL Examples
1. **Lambda-Ready**: Use Lambda optimizations from Session 4
2. **Multi-Account**: Show partner/tenant isolation where relevant
3. **Performance**: Include benchmarks showing speed
4. **Testing**: Unit and integration tests
5. **Documentation**: Clear README with setup instructions
6. **Docker**: Local development with docker-compose
7. **Deployment**: SAM or CDK templates

### Common Structure
```
examples/{app-name}/
├── models/
│   └── models.go          # Domain models
├── handlers/
│   ├── create.go          # Lambda handlers
│   ├── read.go
│   ├── update.go
│   └── delete.go
├── cmd/
│   └── local/
│       └── main.go        # Local testing
├── tests/
│   ├── unit_test.go
│   └── integration_test.go
├── deployment/
│   ├── sam-template.yaml  # SAM deployment
│   └── docker-compose.yml # Local DynamoDB
├── README.md              # Setup & usage guide
└── Makefile              # Build commands
```

## 1. Blog Application (`examples/blog/`)

### Features to Implement
- **Posts**: Create, read, update, delete, publish/draft
- **Comments**: Nested comments with moderation
- **Categories & Tags**: Many-to-many relationships
- **Authors**: User management with roles
- **Search**: Full-text search using DynamoDB patterns

### Models
```go
type Post struct {
    ID          string    `dynamorm:"pk"`
    Slug        string    `dynamorm:"index:gsi-slug,unique"`
    AuthorID    string    `dynamorm:"index:gsi-author"`
    Title       string
    Content     string
    Status      string    `dynamorm:"index:gsi-status-date,pk"`
    PublishedAt time.Time `dynamorm:"index:gsi-status-date,sk"`
    Tags        []string  `dynamorm:"set"`
    ViewCount   int
    Version     int       `dynamorm:"version"`
}

type Comment struct {
    ID       string    `dynamorm:"pk"`
    PostID   string    `dynamorm:"index:gsi-post,pk"`
    ParentID string    `dynamorm:"index:gsi-post,sk,prefix:parent"`
    AuthorID string
    Content  string
    Status   string    // approved, pending, spam
    CreatedAt time.Time
}
```

### Key Patterns to Demonstrate
- Slug-based URLs with unique constraint
- Pagination of posts and comments
- Atomic view counter updates
- Comment threading
- Tag-based filtering

### Lambda Handlers
- `GET /posts` - List with pagination
- `GET /posts/{slug}` - Get by slug
- `POST /posts` - Create post
- `PUT /posts/{id}` - Update post
- `GET /posts/{id}/comments` - Nested resources
- `POST /posts/{id}/comments` - Add comment

## 2. E-commerce (`examples/ecommerce/`)

### Features to Implement
- **Products**: Catalog with variants
- **Cart**: Session-based shopping cart
- **Orders**: Order management workflow
- **Inventory**: Stock tracking with reservations
- **Customers**: User accounts and addresses

### Models
```go
type Product struct {
    ID          string           `dynamorm:"pk"`
    SKU         string           `dynamorm:"index:gsi-sku,unique"`
    CategoryID  string           `dynamorm:"index:gsi-category,pk"`
    Name        string           `dynamorm:"index:gsi-category,sk"`
    Price       int              // cents
    Stock       int
    Variants    []ProductVariant `dynamorm:"json"`
    Version     int              `dynamorm:"version"`
}

type Cart struct {
    ID         string     `dynamorm:"pk"`
    SessionID  string     `dynamorm:"index:gsi-session,unique"`
    CustomerID string     `dynamorm:"index:gsi-customer"`
    Items      []CartItem `dynamorm:"json"`
    ExpiresAt  time.Time  `dynamorm:"ttl"`
    UpdatedAt  time.Time
}

type Order struct {
    ID         string      `dynamorm:"pk"`
    CustomerID string      `dynamorm:"index:gsi-customer,pk"`
    OrderDate  time.Time   `dynamorm:"index:gsi-customer,sk"`
    Status     string      `dynamorm:"index:gsi-status-date,pk"`
    Items      []OrderItem `dynamorm:"json"`
    Total      int
    Version    int         `dynamorm:"version"`
}
```

### Key Patterns to Demonstrate
- Shopping cart with TTL
- Inventory management with optimistic locking
- Order state machine
- Product search and filtering
- Price calculations

### Lambda Handlers
- Product catalog API
- Cart management API
- Checkout process
- Order tracking
- Admin inventory management

## 3. Multi-tenant SaaS (`examples/multi-tenant/`)

### Features to Implement
- **Organizations**: Tenant management
- **Users**: Multi-org user support
- **Projects**: Tenant-scoped resources
- **Billing**: Usage tracking and limits
- **Permissions**: Role-based access control

### Models
```go
type Organization struct {
    ID        string    `dynamorm:"pk"`
    Name      string
    Plan      string    // free, pro, enterprise
    CreatedAt time.Time
    Settings  Settings  `dynamorm:"json"`
}

type User struct {
    ID       string         `dynamorm:"pk"`
    Email    string         `dynamorm:"index:gsi-email,unique"`
    OrgRoles []OrgRole      `dynamorm:"json"`
}

type Project struct {
    ID    string `dynamorm:"pk,composite:org_id,project_id"`
    OrgID string `dynamorm:"extract:org_id"`
    Name  string
    Type  string `dynamorm:"index:gsi-org-type,pk,composite:org_id,type"`
    CreatedAt time.Time `dynamorm:"index:gsi-org-type,sk"`
}

type UsageRecord struct {
    ID        string    `dynamorm:"pk,composite:org_id,timestamp"`
    OrgID     string    `dynamorm:"extract:org_id"`
    Timestamp time.Time `dynamorm:"extract:timestamp"`
    Metric    string
    Value     int
    TTL       time.Time `dynamorm:"ttl"` // 90 days retention
}
```

### Key Patterns to Demonstrate
- Composite keys for tenant isolation
- Cross-tenant user support
- Usage metering and quotas
- Tenant-specific indexes
- Data isolation patterns

### Lambda Handlers
- Organization management
- User invitation flow
- Project CRUD with tenant context
- Usage tracking and billing
- Admin dashboard

## 4. IoT Data Collection (`examples/iot/`)

### Features to Implement
- **Devices**: Registration and management
- **Telemetry**: Time-series data ingestion
- **Alerts**: Rule-based alerting
- **Analytics**: Aggregations and reports
- **Commands**: Device control

### Models
```go
type Device struct {
    ID           string    `dynamorm:"pk"`
    SerialNumber string    `dynamorm:"index:gsi-serial,unique"`
    Type         string    `dynamorm:"index:gsi-type-status,pk"`
    Status       string    `dynamorm:"index:gsi-type-status,sk"`
    Location     Location  `dynamorm:"json"`
    LastSeen     time.Time
    Metadata     map[string]string
}

type Telemetry struct {
    DeviceID  string    `dynamorm:"pk"`
    Timestamp time.Time `dynamorm:"sk"`
    Data      map[string]float64
    TTL       time.Time `dynamorm:"ttl"` // 30 days
}

type Alert struct {
    ID        string    `dynamorm:"pk"`
    DeviceID  string    `dynamorm:"index:gsi-device,pk"`
    Timestamp time.Time `dynamorm:"index:gsi-device,sk"`
    Type      string
    Severity  string
    Message   string
    Resolved  bool
}
```

### Key Patterns to Demonstrate
- Time-series data with sort keys
- TTL for automatic data retention
- Hot partition handling
- Efficient aggregations
- Real-time alerting

### Lambda Handlers
- Device registration API
- Telemetry ingestion (high volume)
- Query API with time ranges
- Alert management
- Batch analytics jobs

## 5. Basic CRUD Tutorial (`examples/basic/`)

### Purpose
A gentle introduction for newcomers showing basic patterns

### Applications to Include
1. **Todo List**: Simple task management
2. **Notes**: Basic note-taking app
3. **Contacts**: Address book
4. **Bookmarks**: URL organizer

### Models (Keep Simple!)
```go
type Todo struct {
    ID        string    `dynamorm:"pk"`
    UserID    string    `dynamorm:"index:gsi-user"`
    Title     string
    Completed bool
    DueDate   time.Time
    CreatedAt time.Time `dynamorm:"created_at"`
    UpdatedAt time.Time `dynamorm:"updated_at"`
}

type Note struct {
    ID        string    `dynamorm:"pk"`
    UserID    string    `dynamorm:"index:gsi-user,pk"`
    Title     string    `dynamorm:"index:gsi-user,sk"`
    Content   string
    Tags      []string  `dynamorm:"set"`
    CreatedAt time.Time `dynamorm:"created_at"`
}
```

### Focus Areas
- Clear, simple code
- Extensive comments
- Step-by-step README
- Common patterns
- Error handling

## Documentation Requirements

### Each README.md Must Include
1. **Overview**: What the example demonstrates
2. **Architecture**: How it's structured
3. **Setup**: Step-by-step local setup
4. **API Reference**: All endpoints documented
5. **Deployment**: How to deploy to AWS
6. **Performance**: Benchmark results
7. **Costs**: Estimated AWS costs
8. **Learning Points**: Key patterns used

### README Template
```markdown
# DynamORM Example: [Name]

## Overview
[Brief description of what this example demonstrates]

## Key Features
- Feature 1
- Feature 2

## Architecture
[Diagram or description]

## Quick Start
1. Clone the repository
2. Run `make setup`
3. Run `make run`

## API Reference
[Document all endpoints]

## Deployment
[SAM/CDK instructions]

## Performance
- Operations/second: X
- Latency p99: Yms
- DynamoDB costs: $Z/month

## What You'll Learn
- Pattern 1: How to...
- Pattern 2: How to...
```

## Testing Requirements

### Unit Tests
- Model validation
- Business logic
- Error cases

### Integration Tests
- Full API flows
- Multi-tenant scenarios
- Performance benchmarks

### Load Tests (where applicable)
- Concurrent users
- Data volume
- Cost projections

## Deployment Templates

### SAM Template Structure
- Lambda functions
- DynamoDB tables with proper indexes
- API Gateway configuration
- IAM roles with least privilege
- CloudWatch alarms

### Local Development
- docker-compose.yml with Local DynamoDB
- Makefile for common tasks
- Environment configuration

## Success Criteria
- [ ] All 5 examples fully functional
- [ ] Lambda deployment tested for each
- [ ] Performance benchmarks included
- [ ] Clear documentation
- [ ] Tests passing
- [ ] Code follows DynamORM best practices
- [ ] Examples showcase different patterns
- [ ] Ready for developers to fork and modify

## Important Notes
1. **Use Session 4's Lambda optimizations** in all examples
2. **Keep examples realistic** - they should be useful templates
3. **Document thoroughly** - assume no DynamoDB knowledge
4. **Test everything** - examples must work out of the box
5. **Coordinate with Team 1** on documentation structure

Remember: These examples are how most developers will learn DynamORM. Make them excellent! 