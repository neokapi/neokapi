module github.com/neokapi/neokapi/bowrain/core

go 1.26.0

require (
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/neokapi/neokapi v0.0.0
	github.com/neokapi/neokapi/bowrain v0.0.0-00010101000000-000000000000
	github.com/neokapi/neokapi/bowrain/plugin/schema v0.0.0-00010101000000-000000000000
	github.com/stretchr/testify v1.11.1
	github.com/zalando/go-keyring v0.2.8
	google.golang.org/protobuf v1.36.11
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/bmatcuk/doublestar/v4 v4.10.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/danieljoos/wincred v1.2.3 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/gabriel-vasile/mimetype v1.4.10 // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	github.com/klauspost/compress v1.18.6 // indirect
	github.com/mattn/go-sqlite3 v1.14.42 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/redis/go-redis/v9 v9.19.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	golang.org/x/net v0.55.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.45.0 // indirect
	golang.org/x/text v0.37.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260420184626-e10c466a9529 // indirect
	google.golang.org/grpc v1.81.1 // indirect
)

replace (
	github.com/neokapi/neokapi => ../..
	github.com/neokapi/neokapi/bowrain => ../
	github.com/neokapi/neokapi/bowrain/plugin/schema => ../plugin/schema
)
