package json

import (
	"testing"
)

// BenchmarkJSONScanner benchmarks tokenizing a moderately complex JSON document.
func BenchmarkJSONScanner(b *testing.B) {
	input := []byte(`{
  "app": {
    "name": "Benchmark Application",
    "version": "2.1.0",
    "description": "A moderately complex JSON document for benchmarking the scanner"
  },
  "messages": {
    "welcome": "Welcome to our application",
    "goodbye": "Thank you for using our service",
    "error_generic": "An unexpected error occurred. Please try again later.",
    "error_network": "Unable to connect to the server. Check your internet connection.",
    "confirm_delete": "Are you sure you want to delete this item?",
    "success_save": "Your changes have been saved successfully.",
    "loading": "Loading, please wait...",
    "empty_state": "No items found. Click the button below to add one."
  },
  "settings": {
    "language": "en-US",
    "theme": "dark",
    "notifications": true,
    "max_retries": 3,
    "timeout": 30.5,
    "features": ["translations", "tm", "qa", null, false],
    "nested": {
      "deeply": {
        "value": "This tests nested object scanning performance"
      }
    }
  },
  "menu": [
    {"id": 1, "label": "File", "shortcut": "Ctrl+F"},
    {"id": 2, "label": "Edit", "shortcut": "Ctrl+E"},
    {"id": 3, "label": "View", "shortcut": "Ctrl+V"},
    {"id": 4, "label": "Help", "shortcut": "F1"}
  ]
}`)

	b.ResetTimer()
	for b.Loop() {
		s := newScanner(input)
		_, _ = s.scan()
	}
}
