package web

import "errors"

// ErrValidation marks a user-input error. Handlers map it to a 4xx response
// rather than a 500. Wrap it with a human message: fmt.Errorf("%w: ...", ErrValidation).
var ErrValidation = errors.New("validation")
