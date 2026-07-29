package usage

import "errors"

var (
	ErrInvalidTenantID  = errors.New("usage: invalid tenant ID")
	ErrInvalidDateRange = errors.New("usage: invalid date range")
)