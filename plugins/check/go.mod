module github.com/neokapi/neokapi/plugins/check

go 1.27.1

require (
	github.com/daulet/tokenizers v1.27.0
	github.com/stretchr/testify v1.12.1
	github.com/yalue/onnxruntime_go v1.30.1
)

require go.yaml.in/yaml/v3 v3.0.5 // indirect

replace github.com/neokapi/neokapi => ../..
