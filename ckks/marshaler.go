package ckks

import (
	"encoding/binary"
	"errors"
	"math"

	"github.com/ldsec/lattigo/v2/ring"
)

// GetDataLen returns the length in bytes of the target Ciphertext.
func (ciphertext *Ciphertext) GetDataLen(WithMetaData bool) (dataLen uint64) {
	// MetaData is :
	// 1 byte : Degree
	// 9 byte : Scale
	// 1 byte : isNTT
	if WithMetaData {
		dataLen += 11
	}

	for _, el := range ciphertext.Value {
		dataLen += el.GetDataLen(WithMetaData)
	}

	return dataLen
}

// MarshalBinary encodes a Ciphertext on a byte slice. The total size
// in byte is 4 + 8* N * numberModuliQ * (degree + 1).
func (ciphertext *Ciphertext) MarshalBinary() (data []byte, err error) {

	data = make([]byte, ciphertext.GetDataLen(true))

	data[0] = uint8(ciphertext.Degree() + 1)

	binary.LittleEndian.PutUint64(data[1:9], math.Float64bits(ciphertext.Scale()))

	if ciphertext.IsNTT {
		data[10] = 1
	}

	var pointer, inc uint64

	pointer = 11

	for _, el := range ciphertext.Value {

		if inc, err = el.WriteTo(data[pointer:]); err != nil {
			return nil, err
		}

		pointer += inc
	}

	return data, nil
}

// UnmarshalBinary decodes a previously marshaled Ciphertext on the target Ciphertext.
// The target Ciphertext must be of the appropriate format and size, it can be created with the
// method NewCiphertext(uint64).
func (ciphertext *Ciphertext) UnmarshalBinary(data []byte) (err error) {
	if len(data) < 11 { // cf. ciphertext.GetDataLen()
		return errors.New("too small bytearray")
	}

	ciphertext.Element = new(Element)

	ciphertext.Value = make([]*ring.Poly, uint8(data[0]))

	ciphertext.scale = math.Float64frombits(binary.LittleEndian.Uint64(data[1:9]))

	if uint8(data[10]) == 1 {
		ciphertext.IsNTT = true
	}

	var pointer, inc uint64
	pointer = 11

	for i := range ciphertext.Value {

		ciphertext.Value[i] = new(ring.Poly)

		if inc, err = ciphertext.Value[i].DecodePolyNew(data[pointer:]); err != nil {
			return err
		}

		pointer += inc
	}

	if pointer != uint64(len(data)) {
		return errors.New("remaining unparsed data")
	}

	return nil
}
