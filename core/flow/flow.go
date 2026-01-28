package flow

import (
	"github.com/asgeirf/gokapi/core/model"
	"github.com/asgeirf/gokapi/core/tool"
)

// Flow represents a configured sequence of Tools that Parts stream through.
type Flow struct {
	Name  string
	Tools []tool.Tool
}

// FlowItem represents a single document to process in a batch.
type FlowItem struct {
	Input          *model.RawDocument
	OutputPath     string
	OutputEncoding string
	TargetLocale   model.LocaleID
}
