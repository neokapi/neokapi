package model_test

import (
	"fmt"

	"github.com/neokapi/neokapi/core/model"
)

func ExampleNewBlock() {
	block := model.NewBlock("b1", "Hello, world!")
	block.SetTargetText("fr", "Bonjour le monde !")

	fmt.Println(block.SourceText())
	fmt.Println(block.TargetText("fr"))
	fmt.Println(block.WordCount())
	// Output:
	// Hello, world!
	// Bonjour le monde !
	// 2
}

func ExampleRunsText() {
	runs := []model.Run{
		{Text: &model.TextRun{Text: "Click "}},
		{PcOpen: &model.PcOpenRun{ID: "1", Type: "a", Data: "<a>"}},
		{Text: &model.TextRun{Text: "here"}},
		{PcClose: &model.PcCloseRun{ID: "1", Type: "a", Data: "</a>"}},
		{Text: &model.TextRun{Text: " to continue"}},
	}
	fmt.Println(model.RunsText(runs))
	// Output:
	// Click here to continue
}
