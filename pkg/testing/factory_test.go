package testing_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/pay-theory/dynamorm/pkg/session"
	dynamormtesting "github.com/pay-theory/dynamorm/pkg/testing"
)

func TestMockDBFactory_CreateDB(t *testing.T) {
	cfg := session.Config{Region: "us-east-1"}
	factory := dynamormtesting.NewMockDBFactory()

	called := false
	factory.OnCreateDB = func(got session.Config) {
		called = true
		require.Equal(t, cfg, got)
	}

	db, err := factory.CreateDB(cfg)
	require.NoError(t, err)
	require.True(t, called)
	require.Equal(t, factory.MockDB, db)

	expectedErr := errors.New("boom")
	factory.WithError(expectedErr)

	db, err = factory.CreateDB(cfg)
	require.ErrorIs(t, err, expectedErr)
	require.Nil(t, db)
}

func TestMockDBFactory_WithMockDB(t *testing.T) {
	factory := dynamormtesting.NewMockDBFactory()
	other := mocks.NewMockExtendedDB()

	returned := factory.WithMockDB(other)
	require.Same(t, factory, returned)
	require.Equal(t, other, factory.MockDB)
}

func TestConfigurableMockDBFactory_EnableLoggingHook(t *testing.T) {
	factory := dynamormtesting.NewConfigurableMockDBFactory()
	require.Nil(t, factory.OnCreateDB)

	factory.WithConfig(dynamormtesting.FactoryConfig{EnableLogging: true})
	_, err := factory.CreateDB(session.Config{})
	require.NoError(t, err)
	require.NotNil(t, factory.OnCreateDB)
}

func TestTestDBFactory_TracksInstances(t *testing.T) {
	factory := &dynamormtesting.TestDBFactory{}

	db1, err := factory.CreateDB(session.Config{})
	require.NoError(t, err)
	require.NotNil(t, db1)

	db2, err := factory.CreateDB(session.Config{})
	require.NoError(t, err)
	require.NotNil(t, db2)

	require.Len(t, factory.Instances, 2)
	require.Equal(t, db2, factory.GetLastInstance())

	factory.Reset()
	require.Empty(t, factory.Instances)
	require.Nil(t, factory.GetLastInstance())
}

func TestTestDBFactory_CreateFunc(t *testing.T) {
	t.Run("tracks on success", func(t *testing.T) {
		expected := mocks.NewMockExtendedDB()

		factory := &dynamormtesting.TestDBFactory{
			CreateFunc: func(_ session.Config) (core.ExtendedDB, error) {
				return expected, nil
			},
		}

		db, err := factory.CreateDB(session.Config{})
		require.NoError(t, err)
		require.Equal(t, expected, db)
		require.Len(t, factory.Instances, 1)
	})

	t.Run("does not track on nil db", func(t *testing.T) {
		factory := &dynamormtesting.TestDBFactory{
			CreateFunc: func(_ session.Config) (core.ExtendedDB, error) {
				return nil, nil
			},
		}

		db, err := factory.CreateDB(session.Config{})
		require.NoError(t, err)
		require.Nil(t, db)
		require.Empty(t, factory.Instances)
	})

	t.Run("does not track on error", func(t *testing.T) {
		expectedErr := errors.New("boom")

		factory := &dynamormtesting.TestDBFactory{
			CreateFunc: func(_ session.Config) (core.ExtendedDB, error) {
				return nil, expectedErr
			},
		}

		db, err := factory.CreateDB(session.Config{})
		require.ErrorIs(t, err, expectedErr)
		require.Nil(t, db)
		require.Empty(t, factory.Instances)
	})
}

func TestSimpleMockFactory(t *testing.T) {
	wasCalled := false

	factory := dynamormtesting.SimpleMockFactory(func(db *mocks.MockExtendedDB) {
		wasCalled = true
		db.On("Close").Return(nil).Once()
	})
	require.True(t, wasCalled)

	db, err := factory.CreateDB(session.Config{})
	require.NoError(t, err)

	mockDB, ok := db.(*mocks.MockExtendedDB)
	require.True(t, ok)

	require.NoError(t, mockDB.Close())
	mockDB.AssertExpectations(t)
}
