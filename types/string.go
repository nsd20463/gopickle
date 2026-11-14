// Copyright 2022 Juniper Networks/Mist Systems. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package types

import (
	"strings"
)

type String interface {
	String() string
	imAString() // since String() is quite common, add a dummy method unlikely to collide
	Object
}

type SimpleString []byte
type EscapedString []byte

func NewString(s []byte, ram *[]byte) Object {
	*ram = s
	// do we need to escape this string?
	for _, r := range s {
		if r < 0x20 || r == '"' || r == '\\' || r >= 0x80 {
			return (*EscapedString)(ram)
		}
	}
	// the entire string is escape-free (a common case)
	// IDEA: since we return Object interface, and that forces the SimpleString to be on the heap, keep a buffer of read-to-go SimpleString
	// objects, or even []byte objects, we can reuse for this, and also List, Dict and friends. This would reduce calls to alloc a huge amount,
	// and not use much extra RAM since we're going to allocate and hold everything in RAM at the end of Load() anyway.
	return (*SimpleString)(ram)
}

func (s *EscapedString) JSON(b *strings.Builder) {
	b.WriteByte('"')
	// the rule in JSON in that JSON text must be UTF-8, or if you must, use unicode \uxxxx notation.
	// only ascii control chars (<0x20), \ and " need to be escaped, and some control chars can use \[bfnrt/] instead of \u00xx encoding.
	// (why / might need to be escaped I don't know, but it's there on json.org's flow chart. The code in stdlib doesn't escaping it, so I won't either)
	for _, r := range string(*s) {
		switch r {
		case '"':
			b.WriteByte('\\')
			b.WriteByte('"')
		case '\\':
			b.WriteByte('\\')
			b.WriteByte('\\')
		case '\u2028':
			b.WriteString("\\u2028")
		case '\u2029':
			b.WriteString("\\u2029")
		case '\b':
			b.WriteByte('\\')
			b.WriteByte('b')
		case '\f':
			b.WriteByte('\\')
			b.WriteByte('f')
		case '\n':
			b.WriteByte('\\')
			b.WriteByte('n')
		case '\r':
			b.WriteByte('\\')
			b.WriteByte('r')
		case '\t':
			b.WriteByte('\\')
			b.WriteByte('t')
		default:
			if r < 0x20 {
				b.WriteString("\\u00")
				b.WriteByte('0' + byte(r>>4))
				b.WriteByte(hex[r&0xf])
			} else {
				// all other runes, even those >= 0x80, can be themselves. JSON source is UTF-8
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
}

const hex = "0123456789abcdef"

// return the string in quotes
func (s *SimpleString) JSON(b *strings.Builder) {
	b.WriteByte('"')
	b.Write(([]byte)(*s))
	b.WriteByte('"')
}

// return the unescaped string (useful every once in a while)
func (s *SimpleString) String() string {
	return string(*s)
}

func (s *EscapedString) String() string {
	return string(*s)
}

func (*SimpleString) imAString()  {}
func (*EscapedString) imAString() {}
