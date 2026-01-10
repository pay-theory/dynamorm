package testing_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/pay-theory/dynamorm/pkg/core"
	dynamormtesting "github.com/pay-theory/dynamorm/pkg/testing"
)

func TestCommonScenarios_SetupTransactionScenario(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		testDB := dynamormtesting.NewTestDB()
		scenarios := dynamormtesting.NewCommonScenarios(testDB)

		scenarios.SetupTransactionScenario(true)

		called := false
		err := testDB.MockDB.Transaction(func(_ *core.Tx) error {
			called = true
			return nil
		})
		require.NoError(t, err)
		require.True(t, called)
	})

	t.Run("failure", func(t *testing.T) {
		testDB := dynamormtesting.NewTestDB()
		scenarios := dynamormtesting.NewCommonScenarios(testDB)

		scenarios.SetupTransactionScenario(false)

		err := testDB.MockDB.Transaction(func(_ *core.Tx) error { return nil })
		require.Error(t, err)
	})
}

func TestCommonScenarios_SetupUpdateBuilder(t *testing.T) {
	testDB := dynamormtesting.NewTestDB()
	scenarios := dynamormtesting.NewCommonScenarios(testDB)
	scenarios.SetupUpdateBuilder()

	builder := testDB.MockQuery.UpdateBuilder()
	require.NotNil(t, builder)

	require.Same(t, builder, builder.Set("name", "alice"))
	require.Same(t, builder, builder.Add("count", 1))
	require.Same(t, builder, builder.Remove("deprecated"))
	require.NoError(t, builder.Execute())
}

func TestMockUpdateBuilder_ReturnTypesAndPanics(t *testing.T) {
	builder := new(dynamormtesting.MockUpdateBuilder)

	t.Run("returns self", func(t *testing.T) {
		builder.On("Set", "field", 1).Return(builder).Once()

		got := builder.Set("field", 1)
		require.Same(t, builder, got)

		builder.AssertExpectations(t)
	})

	t.Run("returns nil", func(t *testing.T) {
		builder.On("Remove", "field").Return(nil).Once()
		require.Nil(t, builder.Remove("field"))
		builder.AssertExpectations(t)
	})

	t.Run("panics on wrong type", func(t *testing.T) {
		builder.On("Add", "field", 1).Return("not a builder").Once()

		require.Panics(t, func() {
			_ = builder.Add("field", 1)
		})

		builder.AssertExpectations(t)
	})

	t.Run("propagates error", func(t *testing.T) {
		expectedErr := errors.New("boom")
		builder.On("ExecuteWithResult", mock.Anything).Return(expectedErr).Once()

		err := builder.ExecuteWithResult(&struct{}{})
		require.ErrorIs(t, err, expectedErr)

		builder.AssertExpectations(t)
	})
}
