module github.com/neokapi/neokapi/scripts/contexteval

go 1.27.0

// contexteval measures how well a model follows the context kapi injects —
// glossary, brand voice, instruction — by translating an engineered corpus twice
// (with and without that context) and scoring both passes with the framework's
// own check tools. Like its sibling batcheval, it sweeps the real models,
// including the one the Bowrain platform actually runs on: AWS Bedrock. The
// Bedrock provider lives in the bowrain module (bowrain/ai/bedrock) precisely so
// aws-sdk-go-v2 never reaches the kapi CLI or Kapi Desktop, which is why this
// eval needs a module of its own — blank-importing bedrock from the framework
// module would break the isolation asserted by `GOWORK=off go build ./...`.
//
// Local modules resolve via go.work.

require (
	github.com/neokapi/neokapi v0.0.0
	github.com/neokapi/neokapi/bowrain v0.0.0
	github.com/stretchr/testify v1.11.1
)
