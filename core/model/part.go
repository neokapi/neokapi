package model

// PartType identifies the kind of Part flowing through a Flow.
type PartType int

const (
	// PartUnknown is the zero value, ensuring uninitialized PartType fields
	// are not silently treated as a valid type.
	PartUnknown PartType = 0

	// Explicit integer values preserve wire compatibility (JSON plugin DTOs,
	// protobuf PartMessage.part_type). Do NOT renumber existing constants.
	PartLayerStart  PartType = 1  // Start of a structural layer
	PartLayerEnd    PartType = 2  // End of a structural layer
	PartGroupStart  PartType = 3  // Start of a structural group within a layer
	PartGroupEnd    PartType = 4  // End of a structural group
	PartBlock       PartType = 5  // Modifiable content
	PartData        PartType = 6  // Non-content document structure
	PartMedia       PartType = 7  // Binary/media content
	_               PartType = 8  // reserved (was PartBatchStart)
	_               PartType = 9  // reserved (was PartBatchEnd)
	_               PartType = 10 // reserved (was PartBatchItemStart)
	_               PartType = 11 // reserved (was PartBatchItemEnd)
	PartRawDocument PartType = 12 // Unprocessed document
	PartCustom      PartType = 13 // Custom extension
)

// String returns the name of the PartType.
func (pt PartType) String() string {
	switch pt {
	case PartUnknown:
		return "Unknown"
	case PartLayerStart:
		return "LayerStart"
	case PartLayerEnd:
		return "LayerEnd"
	case PartGroupStart:
		return "GroupStart"
	case PartGroupEnd:
		return "GroupEnd"
	case PartBlock:
		return "Block"
	case PartData:
		return "Data"
	case PartMedia:
		return "Media"
	case PartRawDocument:
		return "RawDocument"
	case PartCustom:
		return "Custom"
	default:
		return "Unknown"
	}
}

// Part is the fundamental unit of content flowing through a Flow.
// It carries a typed payload (the Resource).
type Part struct {
	Type     PartType
	Resource Resource
}

// PartResult pairs a Part with an optional error, used in channels
// to propagate errors alongside content.
type PartResult struct {
	Part  *Part
	Error error
}
