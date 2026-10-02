package epub

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// A spine item the HTML reader reads keeps a skeleton of its own: the bytes
// around each block, with a reference where the block goes. The EPUB reader
// records it on the item's layer, and the writer replays the item through the
// HTML writer with that skeleton, so every block the write did not change is
// written as the document held it, and an edited one with its inline markup
// and attributes in place.

// memberSkeletonKey names the layer annotation that carries a spine item's
// skeleton.
const memberSkeletonKey = "epub:member-skeleton"

// MemberSkeleton is the skeleton the HTML reader wrote for one spine item, in
// the skeleton store's encoding.
type MemberSkeleton struct {
	Data []byte `json:"data"`
}

// TypeName identifies the annotation.
func (*MemberSkeleton) TypeName() string { return memberSkeletonKey }

func init() {
	model.RegisterPayload(memberSkeletonKey, func() model.Payload { return &MemberSkeleton{} })
}

// wireMemberSkeleton gives the sub-reader a skeleton store when the EPUB's own
// round trip keeps one, and returns it; nil when either side has none.
func (r *Reader) wireMemberSkeleton(subReader format.DataFormatReader) *format.SkeletonStore {
	if r.skeletonStore == nil {
		return nil
	}
	emitter, ok := subReader.(format.SkeletonStoreEmitter)
	if !ok {
		return nil
	}
	store := format.NewMemorySkeletonStore()
	store.SetOriginFormat(subReader.Name())
	emitter.SetSkeletonStore(store)
	return store
}

// recordMemberSkeleton stores the skeleton the sub-reader wrote on the item's
// layer.
func recordMemberSkeleton(layer *model.Layer, store *format.SkeletonStore) {
	if store == nil || store.EntriesWritten() == 0 {
		return
	}
	data, err := store.Bytes()
	if err != nil {
		return
	}
	layer.SetAnno(memberSkeletonKey, &MemberSkeleton{Data: data})
}

// memberSkeletonOf returns the skeleton recorded on a spine item's layer.
func memberSkeletonOf(layer *model.Layer) (*MemberSkeleton, bool) {
	if layer == nil {
		return nil, false
	}
	v, ok := layer.Anno(memberSkeletonKey)
	if !ok {
		return nil, false
	}
	skel, ok := v.(*MemberSkeleton)
	return skel, ok && skel != nil && len(skel.Data) > 0
}

// replayMember writes a spine item through its format's writer and the
// skeleton its reader recorded. ok is false when the format's writer cannot
// consume a skeleton, and the caller takes another path.
func (w *Writer) replayMember(ctx context.Context, layer *model.Layer, skel *MemberSkeleton, parts []*model.Part) (string, bool, error) {
	if w.resolver == nil {
		return "", false, nil
	}
	subWriter, err := w.resolver.ResolveWriter(layer.Format)
	if err != nil {
		return "", false, nil
	}
	consumer, ok := subWriter.(format.SkeletonStoreConsumer)
	if !ok {
		return "", false, nil
	}
	store, err := memberStore(skel.Data)
	if err != nil {
		return "", false, err
	}
	defer store.Close()
	consumer.SetSkeletonStore(store)

	var buf bytes.Buffer
	if err := subWriter.SetOutputWriter(&buf); err != nil {
		return "", false, err
	}
	subWriter.SetLocale(w.Locale)
	ch := make(chan *model.Part, len(parts))
	for _, p := range parts {
		ch <- memberScopedPart(p, layer.ID)
	}
	close(ch)
	if err := subWriter.Write(ctx, ch); err != nil {
		return "", false, err
	}
	if err := subWriter.Close(); err != nil {
		return "", false, err
	}
	return buf.String(), true, nil
}

// memberStore rebuilds a spine item's skeleton for replay, without the bytes
// the HTML reader inserts into a document that declares no charset: an XHTML
// item declares its encoding in its XML declaration, and the item is written
// back with the bytes it held.
func memberStore(data []byte) (*format.SkeletonStore, error) {
	src := format.NewSkeletonStoreFromBytes(data)
	defer src.Close()
	dst := format.NewMemorySkeletonStore()
	for {
		entry, err := src.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			dst.Close()
			return nil, fmt.Errorf("epub: read member skeleton: %w", err)
		}
		switch entry.Type {
		case format.SkeletonText:
			dst.WriteText(entry.Data)
		case format.SkeletonRef:
			dst.WriteRef(string(entry.Data))
		case format.SkeletonLang:
			dst.WriteLang(string(entry.Data))
		case format.SkeletonOriginal:
			if a, b, ok := format.DecodeSkeletonPair(entry.Data); ok {
				dst.WriteOriginal(a, b)
			}
		case format.SkeletonTrimmed:
			if a, b, ok := format.DecodeSkeletonPair(entry.Data); ok {
				dst.WriteTrimmed(a, b)
			}
		case format.SkeletonInserted:
			// Dropped: the item is written with the bytes it held.
		}
	}
	return dst, nil
}

// memberScopedPart returns a part as the spine item's own writer expects it:
// the EPUB reader qualifies each block's id by the item's layer
// (model.QualifyMemberID), and the item's writer matches the ids its reader
// minted. The block is copied, so the qualified id stays on the original.
func memberScopedPart(p *model.Part, member string) *model.Part {
	if p == nil || p.Type != model.PartBlock {
		return p
	}
	b, ok := p.Resource.(*model.Block)
	if !ok {
		return p
	}
	scoped := model.UnqualifyMemberID(b.ID, member)
	if scoped == b.ID {
		return p
	}
	clone := *b
	clone.ID = scoped
	return &model.Part{Type: p.Type, Resource: &clone}
}
