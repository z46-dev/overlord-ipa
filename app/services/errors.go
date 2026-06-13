package services

import "fmt"

const (
	ErrorCodeInvalidInput = "invalid_input"
	ErrorCodeNotFound     = "not_found"
	ErrorCodeUnauthorized = "unauthorized"
	ErrorCodeForbidden    = "forbidden"
	ErrorCodePersistence  = "persistence_error"
	ErrorCodeExecution    = "execution_error"
)

type ServiceError struct {
	Code      string
	Message   string
	Operation string
	Err       error
}

// Error returns a printable service error string.
func (e *ServiceError) Error() (message string) {
	if e.Operation == "" {
		message = e.Message
		return
	}

	message = fmt.Sprintf("%s: %s", e.Operation, e.Message)
	return
}

// Unwrap returns the wrapped error.
func (e *ServiceError) Unwrap() (err error) {
	err = e.Err
	return
}

// NewInvalidInputError creates an invalid input service error.
func NewInvalidInputError(message string, err error) (serviceErr *ServiceError) {
	serviceErr = &ServiceError{Code: ErrorCodeInvalidInput, Message: message, Err: err}
	return
}

// NewPersistenceError creates a persistence service error.
func NewPersistenceError(operation string, err error) (serviceErr *ServiceError) {
	serviceErr = &ServiceError{
		Code:      ErrorCodePersistence,
		Message:   "persistence operation failed",
		Operation: operation,
		Err:       err,
	}
	return
}

// NewNotFoundError creates a not found service error.
func NewNotFoundError(message string, err error) (serviceErr *ServiceError) {
	serviceErr = &ServiceError{Code: ErrorCodeNotFound, Message: message, Err: err}
	return
}

// NewUnauthorizedError creates an unauthorized service error.
func NewUnauthorizedError(message string, err error) (serviceErr *ServiceError) {
	serviceErr = &ServiceError{Code: ErrorCodeUnauthorized, Message: message, Err: err}
	return
}

// NewForbiddenError creates a forbidden service error.
func NewForbiddenError(message string, err error) (serviceErr *ServiceError) {
	serviceErr = &ServiceError{Code: ErrorCodeForbidden, Message: message, Err: err}
	return
}

// NewExecutionError creates an execution service error.
func NewExecutionError(operation string, err error) (serviceErr *ServiceError) {
	serviceErr = &ServiceError{
		Code:      ErrorCodeExecution,
		Message:   "job execution failed",
		Operation: operation,
		Err:       err,
	}
	return
}
