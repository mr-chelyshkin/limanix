package config

import (
	"errors"
	"fmt"

	"github.com/pelletier/go-toml/v2"
)

// Error identifies the invalid field without exposing its value.
// Cause is available to callers through errors.Is and errors.As, but is not
// included in the public diagnostic.
type Error struct {
	Field   string
	Message string
	Cause   error
}

// Error returns the field path and a value-free diagnostic.
func (err *Error) Error() string {
	return err.Field + ": " + err.Message
}

// Unwrap exposes the validation or IO failure to programmatic callers.
func (err *Error) Unwrap() error {
	return err.Cause
}

func fieldError(field, message string) error {
	return &Error{
		Field:   field,
		Message: message,
	}
}

func wrapField(field, message string, cause error) error {
	return &Error{
		Field:   field,
		Message: message,
		Cause:   cause,
	}
}

func decodeError(err error) error {
	message := "invalid TOML"
	if decode, ok := errors.AsType[*toml.DecodeError](err); ok {
		line, column := decode.Position()
		message = fmt.Sprintf("invalid TOML at line %d, column %d", line, column)
	}

	return wrapField("config", message, err)
}
