// Copyright 2020 NLP Odyssey Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package types

import "strings"

// SetAdder is implemented by any value that exhibits a set-like behaviour,
// allowing arbitrary values to be added.
type SetAdder interface {
	Add(Object)
	AddMany([]Object)
	Object
}

// Set represents a Python "set" (builtin type).
type Set []Object

var _ SetAdder = &Set{}

// NewSet makes and returns a new empty Set.
func NewSet(ram *[]Object) *Set {
	s := (*Set)(ram)
	return s
}

// Add adds one element to the Set.
func (s *Set) Add(v Object) {
	*s = append(*s, v)
}

func (s *Set) AddMany(objs []Object) {
	// it's common to load an empty set in one operation
	if len(*s) == 0 {
		*s = objs
		return
	}
	*s = append(*s, objs...)
}

func (s *Set) JSON(b *strings.Builder) {
	b.WriteByte('[')
	for i, o := range *s {
		if i != 0 {
			b.WriteByte(',')
		}
		o.JSON(b)
	}
	b.WriteByte(']')
}
