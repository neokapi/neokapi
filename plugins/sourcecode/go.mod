module github.com/neokapi/neokapi/plugins/sourcecode

go 1.27.1

require (
	github.com/neokapi/neokapi v0.0.0
	github.com/stretchr/testify v1.12.1
	github.com/tdewolff/parse/v2 v2.8.16
	github.com/tree-sitter/go-tree-sitter v0.25.0
	github.com/tree-sitter/tree-sitter-bash v0.25.1
	github.com/tree-sitter/tree-sitter-c v0.24.2
	github.com/tree-sitter/tree-sitter-c-sharp v0.23.5
	github.com/tree-sitter/tree-sitter-cpp v0.23.4
	github.com/tree-sitter/tree-sitter-css v0.25.0
	github.com/tree-sitter/tree-sitter-java v0.23.5
	github.com/tree-sitter/tree-sitter-javascript v0.23.1
	github.com/tree-sitter/tree-sitter-python v0.25.0
	github.com/tree-sitter/tree-sitter-ruby v0.23.1
	github.com/tree-sitter/tree-sitter-rust v0.24.2
	github.com/tree-sitter/tree-sitter-typescript v0.23.2
	google.golang.org/grpc v1.84.0
	mvdan.cc/sh/v3 v3.14.1
)

require (
	github.com/gabriel-vasile/mimetype v1.4.13 // indirect
	github.com/mattn/go-pointer v0.0.1 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/net v0.60.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.42.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260706201446-f0a921348800 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/neokapi/neokapi => ../..
