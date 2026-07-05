package host

import (
	"strings"

	"github.com/neokapi/neokapi/cli/output"
	"github.com/neokapi/neokapi/core/format/schema"
	"github.com/neokapi/neokapi/core/registry"
)

// DeduplicateVersionedFormats removes versioned entries (e.g., "okf_html@2.8.0")
// when a bare-name entry (e.g., "okf_html") exists in the list. This avoids
// showing duplicate entries in `formats list` output.
func DeduplicateVersionedFormats(infos []registry.FormatInfo) []registry.FormatInfo {
	// Build a set of bare names present in the list.
	bareNames := make(map[string]bool, len(infos))
	for _, info := range infos {
		name := string(info.Name)
		if !strings.Contains(name, "@") {
			bareNames[name] = true
		}
	}

	// Filter out versioned entries whose bare name is also present.
	result := make([]registry.FormatInfo, 0, len(infos))
	for _, info := range infos {
		name := string(info.Name)
		if idx := strings.LastIndex(name, "@"); idx > 0 {
			baseName := name[:idx]
			if bareNames[baseName] {
				continue // skip — bare-name alias covers this
			}
		}
		result = append(result, info)
	}
	return result
}

func FilterFormats(infos []registry.FormatInfo, mime, ext string) []registry.FormatInfo {
	mime = strings.ToLower(mime)
	ext = strings.ToLower(ext)
	var result []registry.FormatInfo
	for _, info := range infos {
		if mime != "" && !containsLower(info.MimeTypes, mime) {
			continue
		}
		if ext != "" && !containsLower(info.Extensions, ext) {
			continue
		}
		result = append(result, info)
	}
	return result
}

func containsLower(slice []string, val string) bool {
	for _, s := range slice {
		if strings.ToLower(s) == val {
			return true
		}
	}
	return false
}

func ToFormatInfoParam(name string, prop schema.PropertySchema) output.FormatInfoParam {
	typeStr := prop.Type
	if prop.OkapiFormat != "" {
		typeStr = prop.OkapiFormat
	}
	return output.FormatInfoParam{
		Name:        name,
		Type:        typeStr,
		Default:     prop.Default,
		Description: prop.Description,
	}
}
