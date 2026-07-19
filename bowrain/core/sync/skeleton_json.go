package sync

import (
	"encoding/json"

	"github.com/neokapi/neokapi/core/model"
)

// A block's Skeleton preserves non-translatable document structure for
// write-back. Its Parts are a polymorphic []model.SkeletonPart (SkeletonText |
// SkeletonRef), which plain encoding/json cannot rehydrate to concrete types —
// json.Unmarshal into an interface slice fails outright. Both sync wire
// projections (the proto push's skeleton_json blob and the REST pull's skeleton
// JSON) therefore share this discriminated codec, mirroring the overlay codec,
// so a block carrying skeleton parts round-trips losslessly instead of erroring.

// skeletonWire mirrors model.Skeleton with the polymorphic Parts flattened into
// a discriminated, non-interface shape.
type skeletonWire struct {
	Strategy  int                `json:"strategy"`
	SourceURI string             `json:"sourceUri,omitempty"`
	Parts     []skeletonPartWire `json:"parts,omitempty"`
}

// skeletonPartWire carries either a literal text fragment (kind "text") or a
// resource reference (kind "ref").
type skeletonPartWire struct {
	Kind       string `json:"kind"` // "text" | "ref"
	Text       string `json:"text,omitempty"`
	ResourceID string `json:"resourceId,omitempty"`
	Property   string `json:"property,omitempty"`
	Locale     string `json:"locale,omitempty"`
}

const (
	skeletonPartText = "text"
	skeletonPartRef  = "ref"
)

// MarshalSkeleton encodes a Skeleton (including its polymorphic parts) as JSON.
func MarshalSkeleton(s *model.Skeleton) ([]byte, error) {
	if s == nil {
		return nil, nil
	}
	w := skeletonWire{Strategy: int(s.Strategy), SourceURI: s.SourceURI}
	for _, p := range s.Parts {
		switch v := p.(type) {
		case *model.SkeletonText:
			w.Parts = append(w.Parts, skeletonPartWire{Kind: skeletonPartText, Text: v.Text})
		case *model.SkeletonRef:
			w.Parts = append(w.Parts, skeletonPartWire{
				Kind:       skeletonPartRef,
				ResourceID: v.ResourceID,
				Property:   v.Property,
				Locale:     v.Locale,
			})
		}
	}
	return json.Marshal(w)
}

// UnmarshalSkeleton reverses MarshalSkeleton, reconstructing the concrete
// SkeletonText / SkeletonRef parts by their discriminator.
func UnmarshalSkeleton(data []byte) (*model.Skeleton, error) {
	if len(data) == 0 {
		return nil, nil
	}
	var w skeletonWire
	if err := json.Unmarshal(data, &w); err != nil {
		return nil, err
	}
	s := &model.Skeleton{Strategy: model.SkeletonStrategy(w.Strategy), SourceURI: w.SourceURI}
	for _, pw := range w.Parts {
		switch pw.Kind {
		case skeletonPartRef:
			s.Parts = append(s.Parts, &model.SkeletonRef{ResourceID: pw.ResourceID, Property: pw.Property, Locale: pw.Locale})
		default:
			s.Parts = append(s.Parts, &model.SkeletonText{Text: pw.Text})
		}
	}
	return s, nil
}
