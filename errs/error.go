package errs

import "errors"

type ErrorMessage string

func New(userMessage ErrorMessage) error {

	if userMessage == "" {
		return nil
	}

	return errors.New(string(userMessage))
}

func Build(err error, userMessage ErrorMessage) error {

	if userMessage == "" {
		return err
	}

	return errors.New(err.Error() + "; " + string(userMessage))
}
