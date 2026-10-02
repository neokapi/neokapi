package model_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
)

func BenchmarkVocabularyLoadDefaults(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		reg := model.NewVocabularyRegistry()
		if err := reg.LoadDefaults(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkVocabularyDefault(b *testing.B) {
	model.DefaultVocabulary()
	b.ReportAllocs()
	for b.Loop() {
		model.DefaultVocabulary()
	}
}
