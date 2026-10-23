module github.com/huynguyengl99/chanx-go/layers/redis

go 1.25.0

require (
	github.com/huynguyengl99/chanx-go/layers v0.0.0-00010101000000-000000000000
	github.com/redis/go-redis/v9 v9.22.0
	github.com/vmihailenco/msgpack/v5 v5.4.1
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	go.uber.org/atomic v1.12.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
)

replace github.com/huynguyengl99/chanx-go/layers => ../
