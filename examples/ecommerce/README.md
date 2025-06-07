# DynamORM Example: E-commerce Platform

## Overview

This example demonstrates how to build a scalable e-commerce platform using DynamORM with AWS Lambda. It showcases advanced patterns for building a production-ready online store including:

- Shopping cart with session management and TTL
- Product catalog with variants and inventory tracking
- Order management with state machines
- Customer accounts with addresses and payment methods
- Discount codes and promotions
- Product reviews and ratings
- Wishlist functionality

## Key Features

- **Products**: Full catalog with variants, categories, and inventory
- **Cart**: Session-based cart with automatic expiry (TTL)
- **Orders**: Complete order lifecycle with status tracking
- **Inventory**: Real-time stock tracking with reservations
- **Customers**: Account management with order history
- **Performance**: Optimized for high-traffic e-commerce

## Architecture

```
┌─────────────┐     ┌──────────────┐     ┌─────────────┐
│   Web App   │────▶│ API Gateway  │────▶│   Lambda    │
└─────────────┘     └──────────────┘     └──────┬──────┘
                                                 │
                                          ┌──────▼──────┐
                                          │  DynamoDB   │
                                          │             │
                                          │ Tables:     │
                                          │ - Products  │
                                          │ - Carts     │
                                          │ - Orders    │
                                          │ - Customers │
                                          │ - Inventory │
                                          └─────────────┘
```

### DynamoDB Schema Design

**Products Table**
- PK: `ID`
- GSIs:
  - `gsi-sku`: For SKU lookups
  - `gsi-category`: For category browsing
  
**Carts Table**
- PK: `ID`
- GSIs:
  - `gsi-session`: For session-based cart retrieval
  - `gsi-customer`: For customer cart history
- TTL: `ExpiresAt` (24-hour expiry)

**Orders Table**
- PK: `ID`
- GSIs:
  - `gsi-order-number`: For order lookups
  - `gsi-customer`: For customer order history
  - `gsi-status-date`: For order management

## Quick Start

### Prerequisites
- Go 1.21+
- AWS CLI configured
- Docker (for local DynamoDB)
- Node.js 18+ (for SAM CLI)

### Local Development

1. **Setup environment**:
```bash
cd examples/ecommerce
make setup
```

2. **Start local services**:
```bash
docker-compose up -d
```

3. **Seed sample data**:
```bash
make seed-data
```

4. **Run tests**:
```bash
make test
```

## API Reference

### Cart API

#### Get Cart
```http
GET /cart
X-Session-ID: {session-id}
```

Response:
```json
{
  "success": true,
  "data": {
    "id": "cart-123",
    "items": [
      {
        "product_id": "prod-1",
        "name": "T-Shirt",
        "price": 2999,
        "quantity": 2,
        "subtotal": 5998
      }
    ],
    "subtotal": 5998,
    "tax": 600,
    "total": 6598,
    "expires_at": "2024-01-16T10:00:00Z"
  }
}
```

#### Add to Cart
```http
POST /cart/items
X-Session-ID: {session-id}

{
  "product_id": "prod-1",
  "variant_id": "var-1",
  "quantity": 2
}
```

#### Update Cart Item
```http
PUT /cart/items/{item-id}
X-Session-ID: {session-id}

{
  "quantity": 3
}
```

#### Remove from Cart
```http
DELETE /cart/items/{item-id}
X-Session-ID: {session-id}
```

### Product API

#### List Products
```http
GET /products?category=clothing&limit=20&cursor=xxx
```

Query Parameters:
- `category`: Filter by category ID
- `status`: Filter by status (active, inactive)
- `featured`: Show only featured products
- `search`: Search in name and tags
- `limit`: Results per page (max 100)
- `cursor`: Pagination cursor

#### Get Product
```http
GET /products/{id}
```

Response includes full product details with variants and current stock levels.

#### Search Products
```http
GET /products/search?q=shirt&min_price=1000&max_price=5000
```

### Order API

#### Create Order (Checkout)
```http
POST /orders
Authorization: Bearer {token}

{
  "cart_id": "cart-123",
  "shipping_address": {
    "first_name": "John",
    "last_name": "Doe",
    "address1": "123 Main St",
    "city": "New York",
    "state": "NY",
    "postal_code": "10001",
    "country": "US"
  },
  "payment_method": "card",
  "payment_token": "tok_xxx"
}
```

#### Get Order
```http
GET /orders/{order-number}
Authorization: Bearer {token}
```

#### List Orders
```http
GET /orders?status=processing&limit=20
Authorization: Bearer {token}
```

### Customer API

#### Register Customer
```http
POST /customers/register

{
  "email": "customer@example.com",
  "password": "secure-password",
  "first_name": "John",
  "last_name": "Doe"
}
```

#### Customer Login
```http
POST /customers/login

{
  "email": "customer@example.com",
  "password": "secure-password"
}
```

## Deployment

### Using AWS SAM

1. **Build functions**:
```bash
sam build
```

2. **Deploy**:
```bash
sam deploy --guided
```

### Environment Variables

- `DYNAMODB_REGION`: AWS region
- `STRIPE_SECRET_KEY`: Payment processor key
- `JWT_SECRET`: JWT signing secret
- `CART_TTL_HOURS`: Cart expiry time (default: 24)
- `SESSION_DURATION`: Session duration in seconds

## Performance

### Benchmarks

| Operation | Latency (p99) | Throughput |
|-----------|---------------|------------|
| Add to cart | 20ms | 10,000 req/s |
| Get cart | 15ms | 15,000 req/s |
| Product search | 30ms | 5,000 req/s |
| Create order | 50ms | 2,000 req/s |
| Inventory update | 25ms | 8,000 req/s |

### Optimization Techniques

1. **Session-based Carts**: Reduces database queries
2. **Inventory Reservations**: Prevents overselling
3. **Product Caching**: CloudFront for catalog
4. **Batch Operations**: For bulk updates
5. **GSI Design**: Optimized for common queries

## Cost Estimation

For an e-commerce site with:
- 10,000 products
- 100,000 monthly visitors
- 50,000 carts created
- 10,000 orders
- 5% conversion rate

**Monthly costs**:
- DynamoDB: ~$20-30 (on-demand)
- Lambda: ~$10-15
- API Gateway: ~$35-50
- CloudFront: ~$10-20
- **Total**: ~$75-115/month

## Key Patterns Demonstrated

### 1. Shopping Cart with TTL
- Session-based cart management
- Automatic cleanup of abandoned carts
- Cart persistence across sessions

### 2. Inventory Management
- Real-time stock tracking
- Reservation system during checkout
- Prevent overselling with optimistic locking

### 3. Order State Machine
- Status transitions with validation
- Audit trail for all changes
- Idempotent order processing

### 4. Product Variants
- Size/color combinations
- Per-variant pricing and stock
- Efficient variant queries

### 5. Customer Segmentation
- Tag-based customer groups
- Targeted promotions
- Order history analytics

## Advanced Features

### Discount System
- Percentage and fixed amount discounts
- Product/category specific codes
- Usage limits and expiry dates
- Customer-specific promotions

### Multi-location Inventory
- Track stock across warehouses
- Intelligent fulfillment routing
- Transfer management

### Review System
- Verified purchase reviews
- Rating aggregation
- Merchant responses
- Review moderation

### Wishlist
- Save for later functionality
- Share wishlists
- Price drop notifications
- Stock alerts

## Troubleshooting

### Common Issues

1. **Cart Not Found**: Check session ID is being passed correctly
2. **Out of Stock**: Implement proper reservation system
3. **Payment Failures**: Use idempotency keys
4. **Slow Queries**: Review GSI usage

### Performance Tips

1. Use product search service for complex queries
2. Implement caching for product catalog
3. Batch inventory updates
4. Use DynamoDB streams for async processing

## Security Considerations

1. **PCI Compliance**: Never store card details
2. **Authentication**: Use JWT with short expiry
3. **Rate Limiting**: Prevent abuse
4. **Input Validation**: Sanitize all inputs
5. **HTTPS Only**: Enforce TLS

## Extension Ideas

1. **Recommendations**: ML-based product suggestions
2. **Subscriptions**: Recurring orders
3. **Multi-currency**: International support
4. **B2B Features**: Bulk pricing, net terms
5. **Analytics**: Real-time sales dashboards
6. **Mobile App**: Native app integration
7. **Social Commerce**: Instagram/Facebook shops
8. **Loyalty Program**: Points and rewards

## Contributing

This example provides a foundation for e-commerce applications. Feel free to extend with:
- Additional payment methods
- Shipping integrations
- Tax calculations
- Email notifications
- SMS alerts
- Advanced search with Elasticsearch
- A/B testing framework 