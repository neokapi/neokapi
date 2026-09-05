module github.com/neokapi/neokapi/plugins/vision

go 1.27.0

require (
	github.com/neokapi/neokapi v0.0.0
	github.com/yalue/onnxruntime_go v1.30.1
)

require golang.org/x/text v0.41.0 // indirect

replace github.com/neokapi/neokapi => ../..
