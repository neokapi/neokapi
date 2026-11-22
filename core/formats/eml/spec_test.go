package eml_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/format/spectest"
	"github.com/neokapi/neokapi/core/formats/eml"
	"github.com/stretchr/testify/require"
)

func TestSpec(t *testing.T) {
	s, err := spec.Load("spec.yaml")
	require.NoError(t, err)
	r := spectest.NativeRunner{Spec: s, NewReader: func(string) format.DataFormatReader { return eml.NewReader() }}
	r.Run(t)
}
