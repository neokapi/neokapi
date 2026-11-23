package host

import (
	"os"

	"github.com/neokapi/neokapi/core/project"
)

// preserveAssetVariant reports whether an existing localized binary-asset variant
// at outputPath should be kept instead of reprocessing srcFormat from inputPath.
//
// For binary-asset formats (see project.IsBinaryAssetFormat) the file itself is
// the localizable unit and kapi cannot regenerate a real localization (it can
// only pseudo-localize or copy). So a localized variant already on disk, and
// distinct from the source, is authoritative: reprocessing the source would
// clobber a hand- or connector-supplied translation. A missing variant returns
// false, letting the flow run to produce a fallback. Non-asset (text) formats
// always return false so normal re-translation/merge is unaffected.
func preserveAssetVariant(srcFormat, inputPath, outputPath string) bool {
	return preserveAssetVariantIn("", srcFormat, inputPath, outputPath)
}

// preserveAssetVariantIn is preserveAssetVariant for files of the project at
// root: a path beneath root is stated through it (statIn), so a variant that
// is a link out of the project reads as no variant.
func preserveAssetVariantIn(root, srcFormat, inputPath, outputPath string) bool {
	if !project.IsBinaryAssetFormat(srcFormat) {
		return false
	}
	st, err := statIn(root, outputPath)
	if err != nil || st.IsDir() {
		return false // no existing variant → run the flow
	}
	si, err := statIn(root, inputPath)
	if err != nil {
		return true
	}
	// Source == target (same file): not a distinct variant; let the flow run.
	return !os.SameFile(st, si)
}
