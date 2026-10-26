module go.chanx.dev

go 1.25.0

require (
	github.com/coder/websocket v1.8.15
	github.com/dchest/blake2s v1.0.0
	github.com/goccy/go-yaml v1.19.2
	github.com/google/jsonschema-go v0.4.3
	github.com/gorilla/websocket v1.5.3
	go.chanx.dev/layers v0.1.0
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.3
	go.uber.org/goleak v1.3.0
)

require golang.org/x/text v0.14.0 // indirect

replace go.chanx.dev/layers => ./layers
