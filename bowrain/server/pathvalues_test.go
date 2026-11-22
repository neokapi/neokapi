package server

import "github.com/labstack/echo/v5"

// pathValuesOf pairs route parameter names with their values, in order, for
// Context.SetPathValues in tests that build a context without the router.
func pathValuesOf(names, values []string) echo.PathValues {
	pv := make(echo.PathValues, len(names))
	for i, name := range names {
		pv[i] = echo.PathValue{Name: name, Value: values[i]}
	}
	return pv
}
