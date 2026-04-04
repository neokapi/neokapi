package service

import "errors"

// Sentinel validation errors for the service layer.
var (
	ErrProjectRequired       = errors.New("project is required")
	ErrProjectNameRequired   = errors.New("project name is required")
	ErrSourceLocaleRequired  = errors.New("source locale is required")
	ErrProjectIDRequired     = errors.New("project ID is required")
	ErrBlockIDRequired       = errors.New("block ID is required")
	ErrItemNameRequired      = errors.New("item name is required")
	ErrVersionLabelRequired  = errors.New("version label is required")
	ErrConnectorNameRequired = errors.New("connector name is required")
	ErrConnectorNotFound     = errors.New("connector not found")
)
