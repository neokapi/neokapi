package model

// Side names which run sequence a stand-off interpretation pertains to: the
// source runs, or a target variant. It is used by the flow IO contract (a tool
// produces/consumes a port on the source or target side) and by overlay
// metadata where the distinction is not already carried by Overlay.Variant.
type Side int

const (
	// SideSource: pertains to Block.Source.
	SideSource Side = iota
	// SideTarget: pertains to a target variant.
	SideTarget
)

// String renders the side as the wire token ("source"/"target").
func (s Side) String() string {
	if s == SideTarget {
		return "target"
	}
	return "source"
}

// MarshalText encodes the side as its string token so it is human-readable on
// the wire and in the flow editor.
func (s Side) MarshalText() ([]byte, error) { return []byte(s.String()), nil }

// UnmarshalText decodes the string token form ("source"/"target").
func (s *Side) UnmarshalText(b []byte) error {
	if string(b) == "target" {
		*s = SideTarget
	} else {
		*s = SideSource
	}
	return nil
}
