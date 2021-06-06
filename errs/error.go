package errs

import "errors"

type ErrorMessage string

// New creates a new error object using user message
func New(userMessage ErrorMessage) error {

	if userMessage == "" {
		return nil
	}

	return errors.New(string(userMessage))
}

// Build creates a new error object by appending user message to an error object
func Build(err error, userMessage ErrorMessage) error {

	if userMessage == "" {
		return err
	}

	return errors.New(err.Error() + "; " + string(userMessage))
}
