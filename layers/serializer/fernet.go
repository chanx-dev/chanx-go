package serializer

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"time"
)

// Fernet as specified at github.com/fernet/spec: version 0x80, timestamp, IV,
// AES-128-CBC ciphertext, HMAC-SHA256, urlsafe base64.

// ErrInvalidToken is returned when no key decrypts a message, or it has expired.
var ErrInvalidToken = errors.New("serializer: invalid Fernet token")

const maxClockSkew = 60 * time.Second

type fernetKey struct{ sign, enc []byte }

// deriveKey turns a key into Fernet keys: Fernet(urlsafe_b64(sha256(key))).
func deriveKey(key string) fernetKey {
	sum := sha256.Sum256([]byte(key))
	return fernetKey{sign: sum[:16], enc: sum[16:]}
}

func fernetEncrypt(k fernetKey, msg []byte, now time.Time) ([]byte, error) {
	iv := make([]byte, aes.BlockSize)
	if _, err := rand.Read(iv); err != nil {
		return nil, err
	}
	return fernetEncryptIV(k, msg, now, iv)
}

func fernetEncryptIV(k fernetKey, msg []byte, now time.Time, iv []byte) ([]byte, error) {
	block, err := aes.NewCipher(k.enc)
	if err != nil {
		return nil, err
	}
	pad := aes.BlockSize - len(msg)%aes.BlockSize
	plain := append(bytes.Clone(msg), bytes.Repeat([]byte{byte(pad)}, pad)...)
	ct := make([]byte, len(plain))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ct, plain)

	tok := make([]byte, 0, 1+8+len(iv)+len(ct)+sha256.Size)
	tok = append(tok, 0x80)
	tok = binary.BigEndian.AppendUint64(tok, uint64(now.Unix()))
	tok = append(tok, iv...)
	tok = append(tok, ct...)
	mac := hmac.New(sha256.New, k.sign)
	mac.Write(tok)
	tok = mac.Sum(tok)

	out := make([]byte, base64.URLEncoding.EncodedLen(len(tok)))
	base64.URLEncoding.Encode(out, tok)
	return out, nil
}

// fernetDecrypt tries each key in order; a zero ttl means no age limit.
func fernetDecrypt(keys []fernetKey, token []byte, ttl time.Duration, now time.Time) ([]byte, error) {
	tok := make([]byte, base64.URLEncoding.DecodedLen(len(token)))
	n, err := base64.URLEncoding.Decode(tok, token)
	if err != nil {
		return nil, ErrInvalidToken
	}
	tok = tok[:n]
	if len(tok) < 1+8+aes.BlockSize+sha256.Size || tok[0] != 0x80 {
		return nil, ErrInvalidToken
	}
	ts := time.Unix(int64(binary.BigEndian.Uint64(tok[1:9])), 0)
	if ttl > 0 && ts.Add(ttl).Before(now) {
		return nil, ErrInvalidToken
	}
	if ts.After(now.Add(maxClockSkew)) {
		return nil, ErrInvalidToken
	}
	body, sig := tok[:len(tok)-sha256.Size], tok[len(tok)-sha256.Size:]
	iv, ct := body[9:9+aes.BlockSize], body[9+aes.BlockSize:]
	if len(ct) == 0 || len(ct)%aes.BlockSize != 0 {
		return nil, ErrInvalidToken
	}
	for _, k := range keys {
		mac := hmac.New(sha256.New, k.sign)
		mac.Write(body)
		if subtle.ConstantTimeCompare(mac.Sum(nil), sig) != 1 {
			continue
		}
		block, err := aes.NewCipher(k.enc)
		if err != nil {
			return nil, err
		}
		plain := make([]byte, len(ct))
		cipher.NewCBCDecrypter(block, iv).CryptBlocks(plain, ct)
		pad := int(plain[len(plain)-1])
		if pad == 0 || pad > aes.BlockSize || pad > len(plain) {
			return nil, ErrInvalidToken
		}
		for _, b := range plain[len(plain)-pad:] {
			if int(b) != pad {
				return nil, ErrInvalidToken
			}
		}
		return plain[:len(plain)-pad], nil
	}
	return nil, ErrInvalidToken
}
