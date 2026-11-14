// Copyright 2020 NLP Odyssey Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package types

import "strings"

// FrozenSet represents a Python "frozenset" (builtin type).
type FrozenSet []Object

// NewFrozenSetFromSlice makes and returns a new FrozenSet initialized
// with the elements of the given slice.
func NewFrozenSetFromSlice(slice []Object) FrozenSet {
	return FrozenSet(slice)
}

func (f FrozenSet) JSON(b *strings.Builder) {
	// we represent a FrozenSet as a list in JSON
	b.WriteByte('[')
	for i, o := range f {
		if i != 0 {
			b.WriteByte(',')
		}
		o.JSON(b)
	}
	b.WriteByte(']')
}
