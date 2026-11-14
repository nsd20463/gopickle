// Copyright 2022 Juniper Networks/Mist Systems. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package types

import "strings"

type Bool bool

func NewBool(b bool) Bool {
	return Bool(b)
}

func (v Bool) JSON(b *strings.Builder) {
	if v {
		b.WriteString("true")
	} else {
		b.WriteString("false")
	}
}
