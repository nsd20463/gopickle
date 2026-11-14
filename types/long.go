// Copyright 2022 Juniper Networks/Mist Systems. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package types

import (
	"math/big"
	"strings"
)

type Long big.Int

func NewLong(l *big.Int) *Long {
	return (*Long)(l)
}

func (l *Long) JSON(b *strings.Builder) {
	b.WriteString(((*big.Int)(l)).String())
}
