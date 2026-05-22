package openxml

import "github.com/neokapi/neokapi/core/config"

// okapiOpenXMLTransformer converts OkfOpenxmlFilterConfig specs to OpenxmlFormatConfig.
//
// Okapi's ConditionalParameters uses these key names (camelCase Java fields):
//
//	translateDocumentProperties → translateDocProperties
//	extractNotes               → translateSlideNotes
//	extractMasterPages         → translateSlideMasters
//	extractHiddenSlides        → translateHiddenSlides
//	translateExcelSheetNames   → translateSheetNames
//	translateExcelDrawings     → (dropped — not supported)
//	excludeColors              → excludeColors
//	excludeHighlightColors     → excludeHighlightColors
//	includeHighlightColors     → includeHighlightColors
//	tsExcludedStyles           → excludeStyles
//	tsIncludedStyles           → includeStyles
//	tsExcludedColumns          → excludedColumns
//	tsExcludedSheets           → excludedSheets
//	tsIncludedSlides           → includedSlides
//	tsLineSeparatorReplacement → lineSeparatorReplacement
//	tsReplaceLineSeparator     → replaceLineSeparator
//	tsAggressiveCleanup        → aggressiveCleanup
//	tsTabAsCharacter           → tabAsCharacter
//	tsExtractChartStrings      → translateCharts
//	tsExtractDiagramData       → translateDiagrams
//
// Parameters with no native equivalent are dropped silently.
type okapiOpenXMLTransformer struct{}

func (okapiOpenXMLTransformer) Transform(spec map[string]any) (map[string]any, error) {
	result := make(map[string]any)

	for key, val := range spec {
		switch key {
		// Direct mappings (same key name)
		case "translateDocProperties",
			"translateHiddenText",
			"translateHeadersFooters",
			"translateFootnotes",
			"translateComments",
			"translateHyperlinks",
			"aggressiveCleanup",
			"tabAsCharacter",
			"translateSlideNotes",
			"translateSlideMasters",
			"translateHiddenSlides",
			"translateCharts",
			"translateDiagrams",
			"translateSheetNames",
			"translateSharedStrings",
			"extractRunFontsInfo",
			"replaceLineSeparator",
			"lineSeparatorReplacement",
			"excludeColors",
			"excludeHighlightColors",
			"includeHighlightColors",
			"excludeStyles",
			"includeStyles",
			"excludedSheets",
			"excludedColumns",
			"includedSlides",
			"useCodeFinder",
			"codeFinderRules",
			"fontMappings",
			"complexFieldDefinitionsToExtract":
			result[key] = val

		// Okapi key name → native key name
		case "translateDocumentProperties":
			result["translateDocProperties"] = val
		case "extractNotes":
			result["translateSlideNotes"] = val
		case "extractMasterPages", "extractMasters":
			result["translateSlideMasters"] = val
		case "extractHiddenSlides":
			result["translateHiddenSlides"] = val
		case "translateExcelSheetNames":
			result["translateSheetNames"] = val

		// Okapi "ts" prefixed params → native names
		case "tsExcludedStyles":
			result["excludeStyles"] = val
		case "tsIncludedStyles":
			result["includeStyles"] = val
		case "tsExcludedColumns":
			result["excludedColumns"] = val
		case "tsExcludedSheets":
			result["excludedSheets"] = val
		case "tsIncludedSlides":
			result["includedSlides"] = val
		case "tsLineSeparatorReplacement":
			result["lineSeparatorReplacement"] = val
		case "tsReplaceLineSeparator":
			result["replaceLineSeparator"] = val
		case "tsAggressiveCleanup":
			result["aggressiveCleanup"] = val
		case "tsTabAsCharacter":
			result["tabAsCharacter"] = val
		case "tsExtractChartStrings":
			result["translateCharts"] = val
		case "tsExtractDiagramData":
			result["translateDiagrams"] = val
		case "tsComplexFieldDefinitionsToExtract":
			result["complexFieldDefinitionsToExtract"] = val

		// Okapi composite codeFinder object → extract rules into flat params
		case "codeFinder":
			if m, ok := val.(map[string]any); ok {
				if rules, ok := m["rules"]; ok {
					result["codeFinderRules"] = rules
				}
				result["useCodeFinder"] = true
			}

		// Okapi-only params — drop silently
		case "translateExcelDrawings":
			continue
		// Word Style Optimisation was removed (native is faithful by
		// design — see config.go / the faithful-writer design note). Okapi
		// specs may still carry AllowWordStyleOptimisation → drop it
		// silently rather than erroring in Config.ApplyMap.
		case "optimiseWordStyles", "AllowWordStyleOptimisation":
			continue

		default:
			// Unknown params — pass through for forward compatibility
			result[key] = val
		}
	}

	return result, nil
}

func init() {
	config.DefaultTransforms.Register(
		config.OkapiFilterConfigKind("openxml"), config.FormatConfigKind("openxml"),
		okapiOpenXMLTransformer{},
	)
}
