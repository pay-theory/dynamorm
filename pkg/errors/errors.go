// Package errors defines error types and utilities for DynamORM
package errors

import (
	"errors"
	"fmt"
)

// Common errors that can occur in DynamORM operations
var (
	// ErrItemNotFound is returned when an item is not found in the database
	ErrItemNotFound = errors.New("item not found")

	// ErrInvalidModel is returned when a model struct is invalid
	ErrInvalidModel = errors.New("invalid model")

	// ErrMissingPrimaryKey is returned when a model doesn't have a primary key
	ErrMissingPrimaryKey = errors.New("missing primary key")

	// ErrInvalidPrimaryKey is returned when a primary key value is invalid
	ErrInvalidPrimaryKey = errors.New("invalid primary key")

	// ErrConditionFailed is returned when a condition check fails
	ErrConditionFailed = errors.New("condition check failed")

	// ErrIndexNotFound is returned when a specified index doesn't exist
	ErrIndexNotFound = errors.New("index not found")

	// ErrTransactionFailed is returned when a transaction fails
	ErrTransactionFailed = errors.New("transaction failed")

	// ErrBatchOperationFailed is returned when a batch operation partially fails
	ErrBatchOperationFailed = errors.New("batch operation failed")

	// ErrUnsupportedType is returned when a field type is not supported
	ErrUnsupportedType = errors.New("unsupported type")

	// ErrInvalidTag is returned when a struct tag is invalid
	ErrInvalidTag = errors.New("invalid struct tag")

	// ErrTableNotFound is returned when a table doesn't exist
	ErrTableNotFound = errors.New("table not found")

	// ErrDuplicatePrimaryKey is returned when multiple primary keys are defined
	ErrDuplicatePrimaryKey = errors.New("duplicate primary key definition")

	// ErrEmptyValue is returned when a required value is empty
	ErrEmptyValue = errors.New("empty value")

	// ErrInvalidOperator is returned when an invalid query operator is used
	ErrInvalidOperator = errors.New("invalid query operator")

	// ErrEncryptionNotConfigured is returned when a model uses dynamorm:"encrypted" fields but no KMS key ARN is configured.
	ErrEncryptionNotConfigured = errors.New("encryption not configured")
)

// DynamORMError represents a detailed error with context
type DynamORMError struct {
	Err     error
	Context map[string]any
	Op      string
	Model   string
}

// Error implements the error interface
func (e *DynamORMError) Error() string {
	// SECURITY: Don't expose model names or context data in error messages
	// Only return the operation and underlying error for secure logging
	return fmt.Sprintf("dynamorm: %s operation failed: %v", e.Op, e.Err)
}

// Unwrap returns the underlying error
func (e *DynamORMError) Unwrap() error {
	return e.Err
}

// Is checks if the error matches the target error
func (e *DynamORMError) Is(target error) bool {
	return errors.Is(e.Err, target)
}

// NewError creates a new DynamORMError
func NewError(op, model string, err error) *DynamORMError {
	return &DynamORMError{
		Op:    op,
		Model: model,
		Err:   err,
	}
}

// NewErrorWithContext creates a new DynamORMError with context
func NewErrorWithContext(op, model string, err error, context map[string]any) *DynamORMError {
	return &DynamORMError{
		Op:      op,
		Model:   model,
		Err:     err,
		Context: context,
	}
}

// IsNotFound checks if an error indicates an item was not found
func IsNotFound(err error) bool {
	return errors.Is(err, ErrItemNotFound)
}

// IsInvalidModel checks if an error indicates an invalid model
func IsInvalidModel(err error) bool {
	return errors.Is(err, ErrInvalidModel)
}

// IsConditionFailed checks if an error indicates a condition check failure
func IsConditionFailed(err error) bool {
	return errors.Is(err, ErrConditionFailed)
}

// TransactionError provides context for transactional failures.
type TransactionError struct {
	Err            error
	Operation      string
	Model          string
	Reason         string
	OperationIndex int
}

// Error implements the error interface.
func (e *TransactionError) Error() string {
	if e == nil {
		return "dynamorm: transaction failed"
	}

	op := "transaction"
	if e.Operation != "" {
		op = fmt.Sprintf("%s operation %s", op, e.Operation)
	}
	if e.OperationIndex >= 0 {
		op = fmt.Sprintf("%s (index %d)", op, e.OperationIndex)
	}
	if e.Reason != "" {
		return fmt.Sprintf("dynamorm: %s failed: %s", op, e.Reason)
	}
	return fmt.Sprintf("dynamorm: %s failed", op)
}

// Unwrap returns the underlying error.
func (e *TransactionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}
