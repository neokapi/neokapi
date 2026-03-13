package model

// Package model defines the core content model for the neokapi framework.

// PartType identifies the kind of Part flowing through a Flow.
type PartType int

const (
	PartLayerStart  PartType = iota // Start of a structural layer
	PartLayerEnd                    // End of a structural layer
	PartGroupStart                  // Start of a structural group within a layer
	PartGroupEnd                    // End of a structural group
	PartBlock                       // Translatable content
	PartData                        // Non-translatable document structure
	PartMedia                       // Binary/media content
	_                               // reserved (was PartBatchStart)
	_                               // reserved (was PartBatchEnd)
	_                               // reserved (was PartBatchItemStart)
	_                               // reserved (was PartBatchItemEnd)
	PartRawDocument                 // Unprocessed document
	PartCustom                      // Custom extension
)

// String returns the name of the PartType.
func (pt PartType) String() string {
	switch pt {
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
