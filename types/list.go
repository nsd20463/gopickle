// Copyright 2020 NLP Odyssey Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package types

import (
	"strings"
)

// ListAppender is implemented by any value that exhibits a list-like
// behaviour, allowing arbitrary values to be appended.
type ListAppender interface {
	Append(Object)
	AppendMany([]Object)
	Object
}

// List represents a Python "list" (builtin type).
type List []Object

var _ ListAppender = &List{}

// NewList makes and returns a new empty List.
func NewList(ram *[]Object) *List {
	l := (*List)(ram)
	return l
}

// NewListFromSlice makes and returns a new List initialized with the elements
// of the given slice.
func NewListFromSlice(slice []Object, ram *[]Object) *List {
	l := (*List)(ram)
	*l = slice
	return l
}

// Append appends one element to the end of the List.
func (l *List) Append(obj Object) {
	*l = append(*l, obj)
}

func (l *List) AppendMany(objs []Object) {
	if len(*l) == 0 {
		*l = objs
		return
	}
	*l = append(*l, objs...)
}

func (l *List) JSON(b *strings.Builder) {
	b.WriteByte('[')
	for i, o := range *l {
		if i != 0 {
			b.WriteByte(',')
		}
		o.JSON(b)
	}
	b.WriteByte(']')
}
