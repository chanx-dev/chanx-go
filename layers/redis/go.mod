module go.chanx.dev/layers/redis

go 1.25.0

require (
	github.com/redis/go-redis/v9 v9.22.0
	github.com/vmihailenco/msgpack/v5 v5.4.1
	go.chanx.dev/layers v0.2.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	go.uber.org/atomic v1.12.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
)

replace go.chanx.dev/layers => ../
