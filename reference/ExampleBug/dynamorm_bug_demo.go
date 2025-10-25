package main

import (
	"context"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/pay-theory/dynamorm"
	"github.com/pay-theory/dynamorm/pkg/session"
)

// Model with nested structures and explicit attribute tags
type Merchant struct {
	MerchantUID string   `dynamorm:"pk,attr:merchant_uid" dynamodbav:"merchant_uid"`
	Business    Business `dynamorm:"attr:business" dynamodbav:"business"`
}

type Business struct {
	UnderwritingData UnderwritingData `dynamorm:"attr:underwritingData" dynamodbav:"underwritingData"`
}

type UnderwritingData struct {
	BusinessName string  `dynamorm:"attr:businessName" dynamodbav:"businessName"`
	URL          string  `dynamorm:"attr:url" dynamodbav:"url"`
	MCC          string  `dynamorm:"attr:mcc" dynamodbav:"mcc"`
	Address      Address `dynamorm:"attr:businessAddress" dynamodbav:"businessAddress"`
}

type Address struct {
	City   string `dynamorm:"attr:city" dynamodbav:"city"`
	State  string `dynamorm:"attr:region" dynamodbav:"region"`
	Zip    string `dynamorm:"attr:postalCode" dynamodbav:"postalCode"`
}

func (Merchant) TableName() string {
	return "merchant-onboarding-service-austin-paytheorylab"
}

func main() {
	ctx := context.Background()
	merchantUID := "8b713398-8afb-4b9b-bd47-c07d5c05535e"

	// Load AWS config
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	fmt.Println("=== Testing DynamORM ===")
	testDynamORM(ctx, merchantUID)

	fmt.Println("\n=== Testing AWS SDK attributevalue.UnmarshalMap ===")
	testAWSSDK(ctx, cfg, merchantUID)
}

func testDynamORM(ctx context.Context, merchantUID string) {
	// Initialize DynamORM
	db, err := dynamorm.NewBasic(session.Config{})
	if err != nil {
		log.Fatalf("Failed to init DynamORM: %v", err)
	}

	var merchant Merchant
	merchant.MerchantUID = merchantUID

	err = db.WithContext(ctx).
		Model(&merchant).
		Where("merchant_uid", "=", merchantUID).
		First(&merchant)

	if err != nil {
		fmt.Printf("❌ DynamORM Error: %v\n", err)
		return
	}

	fmt.Printf("Merchant UID: %s\n", merchant.MerchantUID)
	fmt.Printf("Business Name: '%s'\n", merchant.Business.UnderwritingData.BusinessName)
	fmt.Printf("URL: '%s'\n", merchant.Business.UnderwritingData.URL)
	fmt.Printf("MCC: '%s'\n", merchant.Business.UnderwritingData.MCC)
	fmt.Printf("City: '%s'\n", merchant.Business.UnderwritingData.Address.City)

	if merchant.Business.UnderwritingData.BusinessName == "" {
		fmt.Println("❌ FAILED: DynamORM did not unmarshal nested structures")
		fmt.Println("   Issue: converter.go:407 uses field.Name instead of checking dynamodbav/dynamorm tags")
	} else {
		fmt.Println("✅ SUCCESS: DynamORM correctly unmarshaled nested structures")
	}
}

func testAWSSDK(ctx context.Context, cfg aws.Config, merchantUID string) {
	client := dynamodb.NewFromConfig(cfg)

	tableName := "merchant-onboarding-service-austin-paytheorylab"
	result, err := client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: &tableName,
		Key: map[string]types.AttributeValue{
			"merchant_uid": &types.AttributeValueMemberS{Value: merchantUID},
		},
	})

	if err != nil {
		fmt.Printf("❌ AWS SDK Error: %v\n", err)
		return
	}

	var merchant Merchant
	err = attributevalue.UnmarshalMap(result.Item, &merchant)
	if err != nil {
		fmt.Printf("❌ Unmarshal Error: %v\n", err)
		return
	}

	fmt.Printf("Merchant UID: %s\n", merchant.MerchantUID)
	fmt.Printf("Business Name: '%s'\n", merchant.Business.UnderwritingData.BusinessName)
	fmt.Printf("URL: '%s'\n", merchant.Business.UnderwritingData.URL)
	fmt.Printf("MCC: '%s'\n", merchant.Business.UnderwritingData.MCC)
	fmt.Printf("City: '%s'\n", merchant.Business.UnderwritingData.Address.City)

	if merchant.Business.UnderwritingData.BusinessName != "" {
		fmt.Println("✅ SUCCESS: AWS SDK correctly unmarshaled nested structures using dynamodbav tags")
	}
}

func strPtr(s string) *string {
	return &s
}

/*
EXPECTED DynamoDB STRUCTURE:
{
  "merchant_uid": "8b713398-8afb-4b9b-bd47-c07d5c05535e",
  "business": {
    "underwritingData": {
      "businessName": "Paddy's Pub Daycare LLC",
      "url": "www.paddyslittlerascals.com",
      "mcc": "8050",
      "businessAddress": {
        "city": "Philadelphia",
        "region": "PA",
        "postalCode": "19107"
      }
    }
  }
}

BUG DESCRIPTION:
DynamORM's converter (pkg/types/converter.go:407) uses Go field names instead of
checking struct tags when unmarshaling nested Map structures:

    av, exists := m[field.Name]  // Uses "Business" instead of "business"

This causes all nested fields to be empty because:
- DynamoDB has: "business" (lowercase)
- DynamORM looks for: "Business" (capital B from Go field name)
- Tags say: dynamorm:"attr:business" but are ignored

AWS SDK's attributevalue.UnmarshalMap works correctly because it respects
dynamodbav tags for nested structures.
*/
