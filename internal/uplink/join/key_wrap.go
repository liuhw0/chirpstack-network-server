package join

import (
	"fmt"

	"github.com/pkg/errors"

	"github.com/liuhw0/lorawan"
	"github.com/liuhw0/lorawan/backend"
)

// unwrapNSKeyEnveope returns the decrypted key from the given KeyEnvelope.
func unwrapNSKeyEnvelope(ke *backend.KeyEnvelope) (lorawan.AES128Key, error) {
	if ke.KEKLabel == "" {
		var key lorawan.AES128Key
		copy(key[:], ke.AESKey[:])
		return key, nil
	}

	kek, ok := keks[ke.KEKLabel]
	if !ok {
		return lorawan.AES128Key{}, fmt.Errorf("unknown kek label: %s", ke.KEKLabel)
	}

	key, err := ke.Unwrap(kek)
	if err != nil {
		return lorawan.AES128Key{}, errors.Wrap(err, "unwrap error")
	}

	return key, nil
}
