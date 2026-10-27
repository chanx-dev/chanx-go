module go.chanx.dev/adapters/echo/v2

go 1.25.0

require (
	github.com/labstack/echo/v5 v5.4.2
	go.chanx.dev/v2 v2.0.1
)

require (
	github.com/coder/websocket v1.8.15 // indirect
	github.com/dchest/blake2s v1.0.0 // indirect
	github.com/goccy/go-yaml v1.19.2 // indirect
	github.com/google/jsonschema-go v0.4.3 // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.3 // indirect
	go.chanx.dev/layers/v2 v2.0.0 // indirect
	golang.org/x/text v0.40.0 // indirect
)

replace (
	go.chanx.dev/layers/v2 => ../../layers
	go.chanx.dev/v2 => ../../
)
