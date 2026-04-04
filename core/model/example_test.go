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

func ExampleNewFragment() {
	frag := model.NewFragment("Click here to continue")
	fmt.Println(frag.Text())
	fmt.Println(frag.HasSpans())
	// Output:
	// Click here to continue
	// false
}

func ExampleFragment_AppendText() {
	frag := model.NewFragment("Hello")
	frag.AppendText(", world!")
	fmt.Println(frag.Text())
	// Output:
	// Hello, world!
}
