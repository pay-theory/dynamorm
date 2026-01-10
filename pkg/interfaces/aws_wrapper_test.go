package interfaces

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"
)

type failingRoundTripper struct {
	err error
}

func (r failingRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, r.err
}

func newFailingDynamoDBClient(t *testing.T) *dynamodb.Client {
	t.Helper()

	cfg := aws.Config{
		Region:      "us-east-1",
		Credentials: aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider("AKID", "SECRET", "")),
		HTTPClient: &http.Client{
			Transport: failingRoundTripper{err: errors.New("boom")},
		},
		Retryer: func() aws.Retryer {
			return aws.NopRetryer{}
		},
	}

	return dynamodb.NewFromConfig(cfg)
}

func TestDynamoDBClientWrapper_ForwardsCalls(t *testing.T) {
	client := newFailingDynamoDBClient(t)
	wrapper := NewDynamoDBClientWrapper(client)

	ctx := context.Background()

	_, err := wrapper.DescribeTable(ctx, &dynamodb.DescribeTableInput{TableName: aws.String("T")})
	require.Error(t, err)

	_, err = wrapper.Scan(ctx, &dynamodb.ScanInput{TableName: aws.String("T")})
	require.Error(t, err)

	_, err = wrapper.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String("T"),
		Key: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: "1"},
		},
	})
	require.Error(t, err)

	_, err = wrapper.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String("T"),
		Item: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: "1"},
		},
	})
	require.Error(t, err)
}

func TestTableWaiterWrapper_Constructors(t *testing.T) {
	client := newFailingDynamoDBClient(t)

	existsWaiter := NewTableExistsWaiterWrapper(client)
	require.NotNil(t, existsWaiter)

	notExistsWaiter := NewTableNotExistsWaiterWrapper(client)
	require.NotNil(t, notExistsWaiter)
}
