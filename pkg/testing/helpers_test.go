package testing_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pay-theory/dynamorm/pkg/mocks"
	dynamormtesting "github.com/pay-theory/dynamorm/pkg/testing"
)

func TestNewTestDB_CommonSetup(t *testing.T) {
	testDB := dynamormtesting.NewTestDB()
	require.NotNil(t, testDB)
	require.NotNil(t, testDB.MockDB)
	require.NotNil(t, testDB.MockQuery)

	q := testDB.MockDB.Model(struct{}{})
	require.IsType(t, (*mocks.MockQuery)(nil), q)
	mockQuery, ok := q.(*mocks.MockQuery)
	require.True(t, ok)
	require.Same(t, testDB.MockQuery, mockQuery)

	dbWithCtx := testDB.MockDB.WithContext(context.Background())
	require.IsType(t, (*mocks.MockDB)(nil), dbWithCtx)
	mockDB, ok := dbWithCtx.(*mocks.MockDB)
	require.True(t, ok)
	require.Same(t, testDB.MockDB, mockDB)
}

func TestTestDB_ExpectFindCopiesValue(t *testing.T) {
	type user struct {
		ID string
	}

	testDB := dynamormtesting.NewTestDB()

	expected := user{ID: "u1"}
	testDB.ExpectFind(&expected)

	var got user
	err := testDB.MockQuery.First(&got)
	require.NoError(t, err)
	require.Equal(t, expected, got)

	testDB.AssertExpectations(t)
}

func TestTestDB_ExpectAllCopiesValues(t *testing.T) {
	testDB := dynamormtesting.NewTestDB()

	expected := []int{1, 2, 3}
	testDB.ExpectAll(&expected)

	var got []int
	err := testDB.MockQuery.All(&got)
	require.NoError(t, err)
	require.Equal(t, expected, got)

	testDB.AssertExpectations(t)
}

func TestQueryChain_ExpectFirst(t *testing.T) {
	type user struct {
		ID string
	}

	testDB := dynamormtesting.NewTestDB()

	expected := user{ID: "u1"}
	testDB.NewQueryChain().
		Where("id", "=", "u1").
		Limit(10).
		OrderBy("id", "ASC").
		ExpectFirst(&expected)

	var got user
	err := testDB.MockQuery.
		Where("id", "=", "u1").
		Limit(10).
		OrderBy("id", "ASC").
		First(&got)
	require.NoError(t, err)
	require.Equal(t, expected, got)

	testDB.AssertExpectations(t)
}
