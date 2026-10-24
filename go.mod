module github.com/huynguyengl99/chanx-go

go 1.25.0

require (
	github.com/coder/websocket v1.8.15
	github.com/dchest/blake2s v1.0.0
	github.com/goccy/go-yaml v1.19.2
	github.com/google/jsonschema-go v0.4.3
	github.com/gorilla/websocket v1.5.3
	github.com/huynguyengl99/chanx-go/layers v0.0.0-00010101000000-000000000000
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.3
	go.uber.org/goleak v1.3.0
)

require golang.org/x/text v0.14.0 // indirect

replace github.com/huynguyengl99/chanx-go/layers => ./layers
