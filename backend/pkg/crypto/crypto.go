package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

const _keySize = 32

var (
	ErrDecrypt          = errors.New("crypto: ciphertext is invalid or was modified")
	ErrUnknownMasterKey = errors.New("crypto: master key version is not loaded")
)

type MasterKey struct {
	Version uint32
	Key     []byte
}

type WrappedKey struct {
	MasterVersion uint32
	Ciphertext    []byte
}

type Keyring struct {
	current uint32
	keys    map[uint32]cipher.AEAD
}

func NewKeyring(current MasterKey, previous ...MasterKey) (*Keyring, error) {
	k := &Keyring{current: current.Version, keys: make(map[uint32]cipher.AEAD, len(previous)+1)}
	for _, mk := range append([]MasterKey{current}, previous...) {
		aead, err := newAEAD(mk.Key)
		if err != nil {
			return nil, fmt.Errorf("crypto: master key %d: %w", mk.Version, err)
		}
		k.keys[mk.Version] = aead
	}
	return k, nil
}

func (k *Keyring) NewDataKey() WrappedKey {
	key := make([]byte, _keySize)
	_, _ = rand.Read(key)
	return k.wrap(key)
}

func (k *Keyring) Rewrap(w WrappedKey) (WrappedKey, error) {
	key, err := k.unwrap(w)
	if err != nil {
		return WrappedKey{}, err
	}
	return k.wrap(key), nil
}

func (k *Keyring) Seal(w WrappedKey, plaintext, aad []byte) ([]byte, error) {
	aead, err := k.dataAEAD(w)
	if err != nil {
		return nil, err
	}
	return seal(aead, plaintext, aad), nil
}

func (k *Keyring) Open(w WrappedKey, ciphertext, aad []byte) ([]byte, error) {
	aead, err := k.dataAEAD(w)
	if err != nil {
		return nil, err
	}
	return open(aead, ciphertext, aad)
}

// SealEnvelope encrypts plaintext under a fresh data key and returns one blob with the wrapped key and the
// ciphertext, for a column that has no separate key table; masterVersion names the key that wrapped it.
func (k *Keyring) SealEnvelope(plaintext, aad []byte) (blob []byte, masterVersion uint32, err error) {
	w := k.NewDataKey()
	ciphertext, err := k.Seal(w, plaintext, aad)
	if err != nil {
		return nil, 0, err
	}
	n := len(w.Ciphertext)
	if n > math.MaxUint16 {
		return nil, 0, errors.New("crypto: wrapped data key is too long")
	}
	blob = binary.BigEndian.AppendUint16(make([]byte, 0, 2+n+len(ciphertext)), uint16(n))
	blob = append(blob, w.Ciphertext...)
	return append(blob, ciphertext...), w.MasterVersion, nil
}

func (k *Keyring) OpenEnvelope(blob []byte, masterVersion uint32, aad []byte) ([]byte, error) {
	if len(blob) < 2 {
		return nil, ErrDecrypt
	}
	n := int(binary.BigEndian.Uint16(blob))
	if len(blob) < 2+n {
		return nil, ErrDecrypt
	}
	return k.Open(WrappedKey{MasterVersion: masterVersion, Ciphertext: blob[2 : 2+n]}, blob[2+n:], aad)
}

func (k *Keyring) wrap(key []byte) WrappedKey {
	return WrappedKey{MasterVersion: k.current, Ciphertext: seal(k.keys[k.current], key, nil)}
}

func (k *Keyring) unwrap(w WrappedKey) ([]byte, error) {
	master, ok := k.keys[w.MasterVersion]
	if !ok {
		return nil, ErrUnknownMasterKey
	}
	return open(master, w.Ciphertext, nil)
}

func (k *Keyring) dataAEAD(w WrappedKey) (cipher.AEAD, error) {
	key, err := k.unwrap(w)
	if err != nil {
		return nil, err
	}
	return newAEAD(key)
}

func newAEAD(key []byte) (cipher.AEAD, error) {
	if len(key) != _keySize {
		return nil, fmt.Errorf("key must be %d bytes", _keySize)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func seal(aead cipher.AEAD, plaintext, aad []byte) []byte {
	nonce := make([]byte, aead.NonceSize(), aead.NonceSize()+len(plaintext)+aead.Overhead())
	_, _ = rand.Read(nonce)
	return aead.Seal(nonce, nonce, plaintext, aad)
}

func open(aead cipher.AEAD, ciphertext, aad []byte) ([]byte, error) {
	n := aead.NonceSize()
	if len(ciphertext) < n {
		return nil, ErrDecrypt
	}
	plaintext, err := aead.Open(nil, ciphertext[:n], ciphertext[n:], aad)
	if err != nil {
		return nil, ErrDecrypt
	}
	return plaintext, nil
}
