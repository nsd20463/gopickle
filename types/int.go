// Copyright 2022 Juniper Networks/Mist Systems. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package types

import (
	"strconv"
	"strings"
)

type Int int64

func NewInt(i int64) Int {
	return Int(i)
}

func (i Int) JSON(b *strings.Builder) {
	b.WriteString(strconv.FormatInt(int64(i), 10))
}
