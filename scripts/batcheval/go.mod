module github.com/neokapi/neokapi/scripts/batcheval

go 1.27.0

// batcheval sweeps the real models, including the one the Bowrain platform
// actually runs on: AWS Bedrock. The Bedrock provider lives in the bowrain module
// (bowrain/ai/bedrock) precisely so aws-sdk-go-v2 never reaches the kapi CLI or
// Kapi Desktop — every framework AI provider is hand-rolled HTTP for that reason.
//
// So the eval cannot live in the framework module: blank-importing bedrock from
// there would drag the AWS SDK into the framework's go.mod and break the isolation
// the provider was placed in bowrain to protect (asserted by
// `GOWORK=off go build ./...`). Its own module, like gen-refs, keeps both: the eval
// can measure the production path, and the framework stays clean.
//
// Local modules resolve via go.work.

require (
	github.com/neokapi/neokapi v0.0.0
	github.com/neokapi/neokapi/bowrain v0.0.0
	github.com/stretchr/testify v1.11.1
)
