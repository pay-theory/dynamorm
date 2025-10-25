# DynamORM Nested Structure Bug Demo

## Problem Summary

DynamORM's converter (`pkg/types/converter.go:407`) does not respect struct tags when unmarshaling nested Map structures from DynamoDB. It uses Go field names instead of checking `dynamodbav` or `dynamorm` tags.

## The Bug

In `pkg/types/converter.go`, the `mapToStruct` function at line 407:

```go
av, exists := m[field.Name]  // ❌ Uses Go field name "Business"
if !exists {
    continue
}
```

**Should be:**
```go
// Get attribute name from tags first
attrName := getAttributeNameFromTags(field)  // Returns "business" from tags
if attrName == "" {
    attrName = field.Name  // Fallback to field name
}

av, exists := m[attrName]
if !exists {
    continue
}
```

## Impact

Given this struct:
```go
type Merchant struct {
    Business Business `dynamorm:"attr:business" dynamodbav:"business"`
}

type Business struct {
    UnderwritingData UnderwritingData `dynamorm:"attr:underwritingData" dynamodbav:"underwritingData"`
}
```

And this DynamoDB data:
```json
{
  "business": {
    "underwritingData": {
      "businessName": "Test Company"
    }
  }
}
```

**DynamORM looks for:** `Business.UnderwritingData.BusinessName` (capital letters from Go field names)
**DynamoDB has:** `business.underwritingData.businessName` (from the data)
**Tags specify:** `business`, `underwritingData`, `businessName` - but DynamORM ignores these

**Result:** All nested fields are empty/zero values

## Comparison with AWS SDK

AWS SDK's `attributevalue.UnmarshalMap` correctly handles this same structure because it respects `dynamodbav` tags for nested Maps.

## Running the Demo

```bash
cd /tmp
go mod tidy
go run dynamorm_bug_demo.go
```

**Expected Output:**
- ❌ DynamORM returns empty fields for nested structures
- ✅ AWS SDK returns populated fields using the same struct tags

## Test Data

The demo uses merchant UID: `8b713398-8afb-4b9b-bd47-c07d5c05535e` from table `merchant-onboarding-service-austin-paytheorylab`.

This merchant has camelCase attributes in DynamoDB that match the struct tags exactly:
- `business.underwritingData.businessName`: "Paddy's Pub Daycare LLC"
- `business.underwritingData.url`: "www.paddyslittlerascals.com"
- `business.underwritingData.mcc`: "8050"
- `business.underwritingData.businessAddress.city`: "Philadelphia"

## Proposed Fix

The `mapToStruct` function should follow the same pattern used in `executor.go:666-678`:

```go
// Get the dynamodb tag
tag := field.Tag.Get("dynamodb")
if tag == "" {
    tag = field.Tag.Get("dynamorm")
}
if tag == "" || tag == "-" {
    continue
}

// Parse the tag to extract the attribute name
attrName := parseAttributeName(tag)
if attrName == "" {
    attrName = field.Name
}
```

Where `parseAttributeName` extracts `business` from `attr:business` format.
