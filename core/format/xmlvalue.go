package format

import (
	"fmt"

	"github.com/neokapi/neokapi/core/internal/xmlesc"
	"github.com/neokapi/neokapi/core/model"
)

// UnrepresentableValueError reports a block whose text a format cannot
// serialize, naming both the block and the offending character.
//
// It exists because the alternatives are worse. Stripping the character loses
// content the pipeline was asked to carry; emitting it produces a document that
// will not reopen, which surfaces arbitrarily far downstream — a merge, a
// re-extraction, or a customer's editor. A character outside a format's
// representable set reaches a writer only because a tool put it there, so the
// honest response is to name the block and stop.
type UnrepresentableValueError struct {
	// Format is the writer's format id.
	Format string
	// BlockID and BlockName identify the content that carries the character.
	BlockID   string
	BlockName string
	// Locale is the variant being written, empty when writing the source.
	Locale model.LocaleID
	// Err is the format-level reason, e.g. *xmlesc.UnrepresentableCharError.
	Err error
}

func (e *UnrepresentableValueError) Error() string {
	where := e.BlockID
	if e.BlockName != "" {
		where = fmt.Sprintf("%s (%s)", e.BlockID, e.BlockName)
	}
	if !e.Locale.IsEmpty() {
		where = fmt.Sprintf("%s [%s]", where, e.Locale)
	}
	return fmt.Sprintf("%s writer: block %s: %v", e.Format, where, e.Err)
}

func (e *UnrepresentableValueError) Unwrap() error { return e.Err }

// CheckXMLText returns an *UnrepresentableValueError when text a writer is about
// to emit for block carries a character XML 1.0 cannot represent.
//
// A caller passes either a fully rendered fragment — escaping neither
// introduces nor removes control characters, so the check reads the same either
// way — or one text run at a time, which is what [CheckXMLBlock] does.
func CheckXMLText(formatName string, locale model.LocaleID, block *model.Block, rendered string) error {
	err := xmlesc.CheckText(rendered)
	if err == nil {
		return nil
	}
	e := &UnrepresentableValueError{Format: formatName, Locale: locale, Err: err}
	if block != nil {
		e.BlockID = block.ID
		e.BlockName = block.Name
	}
	return e
}

// CheckXMLBlock is CheckXMLText over a block's text runs — its source and each
// target variant. It suits a writer that serializes a block through several
// call sites, where one check at the block boundary covers them all.
//
// Inline-code data is deliberately out of scope. A reader may carry private
// sentinels there — tmx wraps `<sub>` markup in U+0001/U+0002, html replaces a
// translatable attribute value with a U+0000-delimited reference — which the
// writer resolves on the way out, so they never reach the file. Text runs hold
// what a tool wrote, which is what this check is about.
func CheckXMLBlock(formatName string, block *model.Block) error {
	if block == nil {
		return nil
	}
	if err := checkXMLRuns(formatName, "", block, block.Source); err != nil {
		return err
	}
	for key, target := range block.Targets {
		if target == nil {
			continue
		}
		if err := checkXMLRuns(formatName, key.Locale, block, target.Runs); err != nil {
			return err
		}
	}
	return nil
}

func checkXMLRuns(formatName string, locale model.LocaleID, block *model.Block, runs []model.Run) error {
	for _, r := range runs {
		switch r.Kind() {
		case model.RunKindText:
			if err := CheckXMLText(formatName, locale, block, r.Text.Text); err != nil {
				return err
			}
		case model.RunKindPlural:
			for _, form := range r.Plural.Forms {
				if err := checkXMLRuns(formatName, locale, block, form); err != nil {
					return err
				}
			}
		case model.RunKindSelect:
			for _, form := range r.Select.Cases {
				if err := checkXMLRuns(formatName, locale, block, form); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
