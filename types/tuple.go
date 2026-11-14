// Copyright 2020 NLP Odyssey Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package types

import "strings"

// a Tuple can't immediately be a string because we might have to unpack it to use it as arguments to PyNew() or Call()
type Tuple []Object

func NewTupleFromSlice(slice []Object) Tuple {
	return Tuple(slice)
}

func (t Tuple) Len() int         { return len(t) }
func (t Tuple) Get(i int) Object { return t[i] }

func (t Tuple) JSON(b *strings.Builder) {
	b.WriteByte('[')
	for i, o := range t {
		if i != 0 {
			b.WriteByte(',')
		}
		o.JSON(b)
	}
	b.WriteByte(']')
}
