module github.com/huynguyengl99/chanx-go/adapters/echo

go 1.25.0

require (
	github.com/huynguyengl99/chanx-go v0.0.0-00010101000000-000000000000
	github.com/labstack/echo/v5 v5.4.2
)

require (
	github.com/coder/websocket v1.8.15 // indirect
	github.com/dchest/blake2s v1.0.0 // indirect
	github.com/goccy/go-yaml v1.19.2 // indirect
	github.com/google/jsonschema-go v0.4.3 // indirect
	github.com/huynguyengl99/chanx-go/layers v0.0.0-00010101000000-000000000000 // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.3 // indirect
	golang.org/x/text v0.40.0 // indirect
)

replace (
	github.com/huynguyengl99/chanx-go => ../../
	github.com/huynguyengl99/chanx-go/layers => ../../layers
)
