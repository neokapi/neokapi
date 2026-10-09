module github.com/neokapi/neokapi/plugins/sat

go 1.27.1

require (
	github.com/daulet/tokenizers v1.27.0
	github.com/neokapi/neokapi v0.0.0
	github.com/stretchr/testify v1.12.1
	github.com/yalue/onnxruntime_go v1.30.1
	google.golang.org/grpc v1.84.0
)

require (
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/net v0.60.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.42.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260706201446-f0a921348800 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
)

replace github.com/neokapi/neokapi => ../..
