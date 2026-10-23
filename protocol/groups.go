package protocol

import (
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/dchest/blake2s"
)

// MaxGroupNameLength is the longest group name SafeGroupName returns; channel
// layers reject names at or above 100 characters.
const MaxGroupNameLength = 99

const digestLength = 12 // hex characters: a 6-byte blake2s digest

var unsafeGroupChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// SafeGroupName builds a layer-safe group name from arbitrary parts as every
// chanx implementation does, so servers in different languages meet in the same
// groups. Unsupported characters become '-', empty segments are dropped, and a
// name over 99 characters is cut and suffixed with a blake2s digest, keeping
// names that share a prefix apart.
func SafeGroupName(namespace string, parts ...string) (string, error) {
	segments := make([]string, 0, len(parts)+1)
	for _, p := range append([]string{namespace}, parts...) {
		if s := slug(p); s != "" {
			segments = append(segments, s)
		}
	}
	name := strings.Join(segments, ".")
	if name == "" {
		return "", errors.New("protocol: SafeGroupName produced an empty name")
	}
	if len(name) > MaxGroupNameLength {
		h, err := blake2s.New(&blake2s.Config{Size: digestLength / 2})
		if err != nil {
			return "", fmt.Errorf("protocol: blake2s: %w", err)
		}
		h.Write([]byte(name))
		keep := MaxGroupNameLength - digestLength - 1
		name = name[:keep] + "." + hex.EncodeToString(h.Sum(nil))
	}
	return name, nil
}

func slug(s string) string {
	return strings.Trim(unsafeGroupChars.ReplaceAllString(s, "-"), "-")
}
