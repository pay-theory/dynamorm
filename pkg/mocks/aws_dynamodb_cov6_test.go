package mocks_test

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/pay-theory/dynamorm/pkg/mocks"
)

func TestMockDynamoDBClient_AdditionalOperations_COV6(t *testing.T) {
	mockClient := new(mocks.MockDynamoDBClient)
	ctx := context.Background()

	t.Run("query", func(t *testing.T) {
		input := &dynamodb.QueryInput{TableName: aws.String("tbl")}
		mockClient.On("Query", ctx, input, mock.Anything).Return(&dynamodb.QueryOutput{}, nil).Once()

		out, err := mockClient.Query(ctx, input)
		assert.NoError(t, err)
		assert.NotNil(t, out)
	})

	t.Run("scan", func(t *testing.T) {
		input := &dynamodb.ScanInput{TableName: aws.String("tbl")}
		mockClient.On("Scan", ctx, input, mock.Anything).Return(&dynamodb.ScanOutput{}, nil).Once()

		out, err := mockClient.Scan(ctx, input)
		assert.NoError(t, err)
		assert.NotNil(t, out)
	})

	t.Run("update item error returns nil output", func(t *testing.T) {
		input := &dynamodb.UpdateItemInput{TableName: aws.String("tbl")}
		expectedErr := errors.New("update failed")
		mockClient.On("UpdateItem", ctx, input, mock.Anything).Return(nil, expectedErr).Once()

		out, err := mockClient.UpdateItem(ctx, input)
		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, out)
	})

	t.Run("batch get", func(t *testing.T) {
		input := &dynamodb.BatchGetItemInput{}
		mockClient.On("BatchGetItem", ctx, input, mock.Anything).Return(&dynamodb.BatchGetItemOutput{}, nil).Once()

		out, err := mockClient.BatchGetItem(ctx, input)
		assert.NoError(t, err)
		assert.NotNil(t, out)
	})

	t.Run("batch write", func(t *testing.T) {
		input := &dynamodb.BatchWriteItemInput{}
		mockClient.On("BatchWriteItem", ctx, input, mock.Anything).Return(&dynamodb.BatchWriteItemOutput{}, nil).Once()

		out, err := mockClient.BatchWriteItem(ctx, input)
		assert.NoError(t, err)
		assert.NotNil(t, out)
	})

	mockClient.AssertExpectations(t)
}

func TestMockDynamoDBClient_PanicsOnUnexpectedReturnTypes_COV6(t *testing.T) {
	mockClient := new(mocks.MockDynamoDBClient)
	ctx := context.Background()

	input := &dynamodb.QueryInput{TableName: aws.String("tbl")}
	mockClient.On("Query", ctx, input, mock.Anything).Return("bad-type", nil).Once()

	assert.Panics(t, func() {
		_, err := mockClient.Query(ctx, input)
		assert.NoError(t, err)
	})

	mockClient.AssertExpectations(t)
}
