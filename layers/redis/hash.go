package redis

import "hash/crc32"

// consistentHash maps a name onto the shard ring: crc32 to 0..4095, scaled onto
// the hosts, as channels_redis does.
func consistentHash(name string, ringSize int) int {
	if ringSize == 1 {
		return 0
	}
	big := crc32.ChecksumIEEE([]byte(name)) & 0xFFF
	return int(float64(big) / (4096.0 / float64(ringSize)))
}
