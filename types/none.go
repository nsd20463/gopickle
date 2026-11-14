// Copyright 2022 Juniper Networks/Mist Systems. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package types

import "strings"

type None struct{}

func NewNone() None {
	return None{}
}

func (n None) JSON(b *strings.Builder) {
	b.WriteString("null")
}
