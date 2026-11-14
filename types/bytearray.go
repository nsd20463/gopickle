// Copyright 2020 NLP Odyssey Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package types

import (
	"encoding/base64"
	"strings"
)

// ByteArray represents a Python "bytearray" (builtin type).
type ByteArray []byte

// NewByteArray makes and returns a new ByteArray initialized with
// the elements of the given slice.
func NewByteArray(bytes []byte) ByteArray {
	return ByteArray(bytes)
}

func (a ByteArray) JSON(b *strings.Builder) {
	b.WriteByte('"')
	if len(a) < 1024*3/4 { // encoder's built-in buffer is 1024 bytes
		b.Write(base64.StdEncoding.AppendEncode(nil, []byte(a)))
	} else {
		// stream the larger values
		w := base64.NewEncoder(base64.StdEncoding, b)
		w.Write([]byte(a))
		w.Close()
	}
	b.WriteByte('"')
}
