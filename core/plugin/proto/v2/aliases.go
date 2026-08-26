// Package bridgev2 hosts the generated bridge service types. The
// content-model messages themselves moved to the canonical wire schema in
// core/proto/content/v1 (package contentv1); the aliases below keep existing
// bridgev2 references (plugin hosts, parity harness, plugin binaries)
// compiling unchanged. New code should import contentv1 directly.
package bridgev2

import (
	contentv1 "github.com/neokapi/neokapi/core/proto/content/v1"
)

// Content-model message aliases (canonical definitions in contentv1).
type (
	AnnotationEntry       = contentv1.AnnotationEntry
	AnchorMessage         = contentv1.AnchorMessage
	RunPosMessage         = contentv1.RunPosMessage
	RunPathStepMessage    = contentv1.RunPathStepMessage
	VariantMessage        = contentv1.VariantMessage
	SpanMessage           = contentv1.SpanMessage
	OverlayMessage        = contentv1.OverlayMessage
	RunConstraints        = contentv1.RunConstraints
	TextRunMessage        = contentv1.TextRunMessage
	PlaceholderRunMessage = contentv1.PlaceholderRunMessage
	PcOpenRunMessage      = contentv1.PcOpenRunMessage
	PcCloseRunMessage     = contentv1.PcCloseRunMessage
	SubRunMessage         = contentv1.SubRunMessage
	PluralRunMessage      = contentv1.PluralRunMessage
	SelectRunMessage      = contentv1.SelectRunMessage
	RunList               = contentv1.RunList
	RunMessage            = contentv1.RunMessage
	SegmentMessage        = contentv1.SegmentMessage
	TargetEntry           = contentv1.TargetEntry
	ContentBlock          = contentv1.ContentBlock
	SkeletonMessage       = contentv1.SkeletonMessage
	SkeletonPartMessage   = contentv1.SkeletonPartMessage
	DisplayHintMessage    = contentv1.DisplayHintMessage
	BlockMessage          = contentv1.BlockMessage
	LayerMessage          = contentv1.LayerMessage
	DataMessage           = contentv1.DataMessage
	GroupStartMessage     = contentv1.GroupStartMessage
	GroupEndMessage       = contentv1.GroupEndMessage
	MediaMessage          = contentv1.MediaMessage
	PartMessage           = contentv1.PartMessage
	ContentRef            = contentv1.ContentRef
)

// Oneof wrapper aliases. The names must match the generated protobuf
// wrapper names exactly, underscores included.
//
//nolint:staticcheck // ST1003: generated-style protobuf names
type (
	RunMessage_Text    = contentv1.RunMessage_Text
	RunMessage_Ph      = contentv1.RunMessage_Ph
	RunMessage_PcOpen  = contentv1.RunMessage_PcOpen
	RunMessage_PcClose = contentv1.RunMessage_PcClose
	RunMessage_Sub     = contentv1.RunMessage_Sub
	RunMessage_Plural  = contentv1.RunMessage_Plural
	RunMessage_Select  = contentv1.RunMessage_Select
	ContentRef_Inline  = contentv1.ContentRef_Inline
	ContentRef_Path    = contentv1.ContentRef_Path
	ContentRef_Uri     = contentv1.ContentRef_Uri
)
