package xliff2

import (
	"sort"
	"strings"

	"github.com/beevik/etree"

	"github.com/neokapi/neokapi/core/model"
)

// A <unit> may carry the XLIFF 2 Metadata module: <mda:metadata> holding
// <mda:metaGroup category="…"> groups of <mda:meta type="…">value</mda:meta>.
// The reader keeps each meta as a block property keyed
// UnitMetaKey(category, type), and the writer emits every such property back
// as the unit's metadata, so a unit's metadata survives a round trip through
// the content model and a block can be given metadata to write.

// MetadataNamespace is the namespace of the XLIFF 2 Metadata module.
const MetadataNamespace = "urn:oasis:names:tc:xliff:metadata:2.0"

// UnitMetaPropertyPrefix starts the block property that holds one meta of a
// unit's metadata.
const UnitMetaPropertyPrefix = "mda:"

// UnitMetaKey is the block property that holds the meta of type typ in the
// metaGroup of category.
func UnitMetaKey(category, typ string) string {
	return UnitMetaPropertyPrefix + category + ":" + typ
}

// The metadata kapi extract writes on each unit, in the metaGroup of category
// FileNoteCategoryKapi, and kapi merge reads back: the revision of the target
// edition the unit was extracted against, and the revision of the source the
// unit's source was read from (model.EditionRevision).
const (
	UnitMetaIfMatch = "if-match"
	UnitMetaBasis   = "basis"
)

// readUnitMetadata records the <mda:metadata> of unit on block.
func readUnitMetadata(unit *etree.Element, block *model.Block) {
	for _, child := range unit.ChildElements() {
		if child.Tag != "metadata" || !(isMetadataSpace(child.NamespaceURI()) || isMetadataSpace(child.Space)) {
			continue
		}
		readMetaGroups(child, "", block)
	}
}

// isMetadataSpace reports whether space names the Metadata module: its
// namespace, or the mda prefix a document uses without declaring it.
func isMetadataSpace(space string) bool {
	return space == MetadataNamespace || space == "mda"
}

// readMetaGroups records each meta under el, a group of category or the
// metadata element itself.
func readMetaGroups(el *etree.Element, category string, block *model.Block) {
	for _, c := range el.ChildElements() {
		switch c.Tag {
		case "metaGroup":
			readMetaGroups(c, attrValue(c, "category"), block)
		case "meta":
			typ := attrValue(c, "type")
			if typ == "" {
				continue
			}
			if block.Properties == nil {
				block.Properties = map[string]string{}
			}
			block.Properties[UnitMetaKey(category, typ)] = strings.TrimSpace(c.Text())
		}
	}
}

// appendUnitMetadata writes block's metadata properties as the unit's
// <mda:metadata>, one metaGroup per category in name order, each meta in type
// order. A block with none writes nothing.
func appendUnitMetadata(unit *etree.Element, block *model.Block) {
	groups := map[string]map[string]string{}
	for k, v := range block.Properties {
		rest, ok := strings.CutPrefix(k, UnitMetaPropertyPrefix)
		if !ok {
			continue
		}
		category, typ, ok := strings.Cut(rest, ":")
		if !ok || typ == "" {
			continue
		}
		if groups[category] == nil {
			groups[category] = map[string]string{}
		}
		groups[category][typ] = v
	}
	if len(groups) == 0 {
		return
	}
	md := unit.CreateElement("mda:metadata")
	md.CreateAttr("xmlns:mda", MetadataNamespace)
	for _, category := range sortedKeys(groups) {
		g := md.CreateElement("mda:metaGroup")
		if category != "" {
			g.CreateAttr("category", category)
		}
		for _, typ := range sortedKeys(groups[category]) {
			m := g.CreateElement("mda:meta")
			m.CreateAttr("type", typ)
			m.SetText(groups[category][typ])
		}
	}
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
