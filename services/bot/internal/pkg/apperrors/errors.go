package apperrors

import "fmt"

func New(methodCtx string, message string) error {
	return fmt.Errorf("%s: %s", methodCtx, message)
}

func Wrap(methodCtx string, err error) error {
	if err == nil {
		return nil
	}

	return fmt.Errorf("%s: %w", methodCtx, err)
}
