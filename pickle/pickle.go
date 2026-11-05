// Copyright 2020 NLP Odyssey Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pickle

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"strconv"
	"unsafe"

	"github.com/mistsys/gopickle2json/types"
)

const HighestProtocol byte = 5

type Unpickler struct {
	in             []byte // unread input
	currentFrame   []byte // nil, or unread portion of current frame
	stack          []types.Object
	metaStack      [][]types.Object
	memo           map[uint32]types.Object
	ram            []types.Object
	sram           [][]byte
	dram           [][]types.Object
	FindClass      func(module, name string) (types.Object, error)
	PersistentLoad func(types.Object) (types.Object, error)
	GetExtension   func(code int) (types.Object, error)
	NextBuffer     func() (types.Object, error)
	MakeReadOnly   func(types.Object) (types.Object, error)
	proto          byte
}

func NewUnpickler(in []byte) Unpickler {
	return Unpickler{
		in: in,
	}
}

func (u *Unpickler) Load() (types.Object, error) {
	u.metaStack = make([][]types.Object, 0, 16)
	u.memo = make(map[uint32]types.Object, 256+128)
	defer func(u *Unpickler) {
		u.in = nil
		u.currentFrame = nil
		u.stack = nil
		u.metaStack = nil
		u.memo = nil
		u.ram = nil
		u.sram = nil
		u.dram = nil
		u.proto = 0
	}(u)

	for {
		opcode, err := u.readOne()
		if err != nil {
			return nil, err
		}

		opFunc := dispatch[opcode]
		if opFunc == nil {
			return nil, fmt.Errorf("unknown opcode: 0x%x '%c'", opcode, opcode)
		}

		err = opFunc(u)
		if err != nil {
			if p, ok := err.(pickleStop); ok {
				return p.value, nil
			}
			return nil, err
		}
	}
}

type pickleStop struct{ value types.Object }

func (p pickleStop) Error() string { return "STOP" }

var _ error = pickleStop{}

func (u *Unpickler) alloc_sram() *[]byte {
	if len(u.sram) == 0 {
		u.sram = make([][]byte, 256)
		u.sram = u.sram[:cap(u.sram)]
	}
	alloc := &u.sram[0]
	u.sram = u.sram[1:]
	return alloc
}

func (u *Unpickler) alloc_dram() *[]types.Object {
	if len(u.dram) == 0 {
		u.dram = make([][]types.Object, 256)
		u.dram = u.dram[:cap(u.dram)]
	}
	alloc := &u.dram[0]
	u.dram = u.dram[1:]
	return alloc
}

func (u *Unpickler) NewString(s []byte) types.Object {
	return types.NewString(s, u.alloc_sram())
}

func (u *Unpickler) findClass(module, name string) (types.Object, error) {
	switch module {
	case "collections":
		switch name {
		case "OrderedDict":
			return &types.OrderedDictClass{}, nil
		}

	case "__builtin__":
		switch name {
		case "object":
			return &types.ObjectClass{}, nil
		}
	}
	if u.FindClass != nil {
		return u.FindClass(module, name)
	}
	panic(fmt.Sprintf("can't unpickle type %s.%s", string(module), string(name)))
}

func (u *Unpickler) read(n int) ([]byte, error) {
	var out []byte

	if u.currentFrame != nil {
		if len(u.currentFrame) < n {
			if len(u.currentFrame) == 0 {
				u.currentFrame = nil
				goto no_current_frame
			}
			return nil, io.ErrUnexpectedEOF
		}
		out, u.currentFrame = u.currentFrame[:n:n], u.currentFrame[n:]
		return out, nil
	}
no_current_frame:

	if len(u.in) < n {
		return nil, io.ErrUnexpectedEOF
	}
	out, u.in = u.in[:n:n], u.in[n:]
	return out, nil
}

func (u *Unpickler) readOne() (byte, error) {
	var b byte
	if len(u.currentFrame) != 0 {
		b, u.currentFrame = u.currentFrame[0], u.currentFrame[1:]
		return b, nil
	}
	if len(u.in) == 0 {
		return 0, io.ErrUnexpectedEOF
	}
	b, u.in = u.in[0], u.in[1:]
	return b, nil
}

// return the line as a []byte, without the terminating \n (which is required to be present)
func (u *Unpickler) readLineBytes() ([]byte, error) {
	var out []byte
	var err error
	if len(u.currentFrame) != 0 {
		out, u.currentFrame, err = readLine(u.currentFrame)
	} else {
		out, u.in, err = readLine(u.in)
	}
	return out, err
}

// return the line as an unsafe string, without the terminating \n (which is required to be present)
// The caller MUST NOT do anything which would hold onto the string after the current call to Unpickler.Load() finishes.
// In return, this avoids copying a []byte to a string buffer.
func (u *Unpickler) unsafeReadLine() (string, error) {
	var out []byte
	var err error
	if len(u.currentFrame) != 0 {
		out, u.currentFrame, err = readLine(u.currentFrame)
	} else {
		out, u.in, err = readLine(u.in)
	}
	// construct the string without copying. we hold the caller to the promise that they don't store the returned string
	// after the current Unpickler.Load() call
	return *(*string)(unsafe.Pointer(&out)), err
}

func readLine(in []byte) (line []byte, out []byte, err error) {
	for i, b := range in {
		if b == '\n' {
			line, out = in[:i:i], in[i+1:] // note we skip and don't return the \n; no callers want it
			return line, out, nil
		}
	}
	return nil, in, io.ErrUnexpectedEOF
}

func (u *Unpickler) loadFrame(frameSize int) error {
	if len(u.currentFrame) != 0 {
		return errors.New("beginning of a new frame before end of current frame")
	}
	if len(u.in) < frameSize {
		return io.ErrUnexpectedEOF
	}
	u.currentFrame, u.in = u.in[:frameSize:frameSize], u.in[frameSize:]
	return nil
}

func (u *Unpickler) append(element types.Object) {
	u.stack = append(u.stack, element)
}

func (u *Unpickler) stackLast() (types.Object, error) {
	if len(u.stack) == 0 {
		return nil, fmt.Errorf("the stack is empty")
	}
	return u.stack[len(u.stack)-1], nil
}

func (u *Unpickler) stackPop() (types.Object, error) {
	element, err := u.stackLast()
	if err != nil {
		return nil, err
	}
	u.stack = u.stack[:len(u.stack)-1]
	return element, nil
}

func (u *Unpickler) metaStackLast() ([]types.Object, error) {
	if len(u.metaStack) == 0 {
		return nil, fmt.Errorf("the meta stack is empty")
	}
	return u.metaStack[len(u.metaStack)-1], nil
}

func (u *Unpickler) metaStackPop() ([]types.Object, error) {
	element, err := u.metaStackLast()
	if err != nil {
		return nil, err
	}
	u.metaStack = u.metaStack[:len(u.metaStack)-1]
	return element, nil
}

// Returns a list of items pushed in the stack after last MARK instruction.
func (u *Unpickler) popMark() ([]types.Object, error) {
	items := u.stack
	newStack, err := u.metaStackPop()
	if err != nil {
		return nil, err
	}
	u.stack = newStack
	return items, nil
}

var dispatch [math.MaxUint8]func(*Unpickler) error

func init() {
	// Initialize `dispatch` assigning functions to opcodes

	// Protocol 0 and 1

	dispatch['('] = loadMark
	dispatch['.'] = loadStop
	dispatch['0'] = loadPop
	dispatch['1'] = loadPopMark
	dispatch['2'] = loadDup
	dispatch['F'] = loadFloat
	dispatch['I'] = loadInt
	dispatch['J'] = loadBinInt
	dispatch['K'] = loadBinInt1
	dispatch['L'] = loadLong
	dispatch['M'] = loadBinInt2
	dispatch['N'] = loadNone
	dispatch['P'] = loadPersId
	dispatch['Q'] = loadBinPersId
	dispatch['R'] = loadReduce
	dispatch['S'] = loadString
	dispatch['T'] = loadBinString
	dispatch['U'] = loadShortBinString
	dispatch['V'] = loadUnicode
	dispatch['X'] = loadBinUnicode
	dispatch['a'] = loadAppend
	dispatch['b'] = loadBuild
	dispatch['c'] = loadGlobal
	dispatch['d'] = loadDict
	dispatch['}'] = loadEmptyDict
	dispatch['e'] = loadAppends
	dispatch['g'] = loadGet
	dispatch['h'] = loadBinGet
	dispatch['i'] = loadInst
	dispatch['j'] = loadLongBinGet
	dispatch['l'] = loadList
	dispatch[']'] = loadEmptyList
	dispatch['o'] = loadObj
	dispatch['p'] = loadPut
	dispatch['q'] = loadBinPut
	dispatch['r'] = loadLongBinPut
	dispatch['s'] = loadSetItem
	dispatch['t'] = loadTuple
	dispatch[')'] = loadEmptyTuple
	dispatch['u'] = loadSetItems
	dispatch['G'] = loadBinFloat

	// Protocol 2

	dispatch['\x80'] = loadProto
	dispatch['\x81'] = loadNewObj
	dispatch['\x82'] = opExt1
	dispatch['\x83'] = opExt2
	dispatch['\x84'] = opExt4
	dispatch['\x85'] = loadTuple1
	dispatch['\x86'] = loadTuple2
	dispatch['\x87'] = loadTuple3
	dispatch['\x88'] = loadTrue
	dispatch['\x89'] = loadFalse
	dispatch['\x8a'] = loadLong1
	dispatch['\x8b'] = loadLong4

	// Protocol 3 (Python 3.x)

	dispatch['B'] = loadBinBytes
	dispatch['C'] = loadShortBinBytes

	// Protocol 4

	dispatch['\x8c'] = loadShortBinUnicode
	dispatch['\x8d'] = loadBinUnicode8
	dispatch['\x8e'] = loadBinBytes8
	dispatch['\x8f'] = loadEmptySet
	dispatch['\x90'] = loadAddItems
	dispatch['\x91'] = loadFrozenSet
	dispatch['\x92'] = loadNewObjEx
	dispatch['\x93'] = loadStackGlobal
	dispatch['\x94'] = loadMemoize
	dispatch['\x95'] = loadFrame

	// Protocol 5

	dispatch['\x96'] = loadByteArray8
	dispatch['\x97'] = loadNextBuffer
	dispatch['\x98'] = loadReadOnlyBuffer
}

// identify pickle protocol
func loadProto(u *Unpickler) error {
	proto, err := u.readOne()
	if err != nil {
		return err
	}
	if proto > HighestProtocol {
		return fmt.Errorf("unsupported pickle protocol: %d", proto)
	}
	u.proto = proto
	return nil
}

// indicate the beginning of a new frame
func loadFrame(u *Unpickler) error {
	buf, err := u.read(8)
	if err != nil {
		return err
	}
	frameSize := binary.LittleEndian.Uint64(buf)
	if frameSize > math.MaxInt64 {
		return fmt.Errorf("frame size > max int64: %d", frameSize)
	}
	return u.loadFrame(int(frameSize))
}

// push persistent object; id is taken from string arg
func loadPersId(u *Unpickler) error {
	if u.PersistentLoad == nil {
		return fmt.Errorf("unsupported persistent ID encountered")
	}
	line, err := u.readLineBytes()
	if err != nil {
		return err
	}
	result, err := u.PersistentLoad(u.NewString(line))
	if err != nil {
		return err
	}
	u.append(result)
	return nil
}

// push persistent object; id is taken from stack
func loadBinPersId(u *Unpickler) error {
	if u.PersistentLoad == nil {
		return fmt.Errorf("unsupported persistent ID encountered")
	}
	pid, err := u.stackPop()
	if err != nil {
		return err
	}
	result, err := u.PersistentLoad(pid)
	if err != nil {
		return err
	}
	u.append(result)
	return nil
}

// push None (nil)
func loadNone(u *Unpickler) error {
	u.append(types.NewNone())
	return nil
}

// push False
func loadFalse(u *Unpickler) error {
	u.append(types.NewBool(false))
	return nil
}

// push True
func loadTrue(u *Unpickler) error {
	u.append(types.NewBool(true))
	return nil
}

// push integer or bool; decimal string argument
func loadInt(u *Unpickler) error {
	data, err := u.unsafeReadLine()
	if err != nil {
		return err
	}
	if len(data) == 2 && data[0] == '0' && data[1] == '0' {
		u.append(types.NewBool(false))
		return nil
	}
	if len(data) == 2 && data[0] == '0' && data[1] == '1' {
		u.append(types.NewBool(true))
		return nil
	}
	i, err := strconv.ParseInt(data, 10, 64)
	if err != nil {
		return err
	}
	u.append(types.NewInt(i))
	return nil
}

// push four-byte signed int
func loadBinInt(u *Unpickler) error {
	buf, err := u.read(4)
	if err != nil {
		return err
	}
	u.append(types.NewInt(int64(decodeInt32(buf))))
	return nil
}

// push 1-byte unsigned int
func loadBinInt1(u *Unpickler) error {
	i, err := u.readOne()
	if err != nil {
		return err
	}
	u.append(types.NewInt(int64(i)))
	return nil
}

// push 2-byte unsigned int
func loadBinInt2(u *Unpickler) error {
	buf, err := u.read(2)
	if err != nil {
		return err
	}
	u.append(types.NewInt(int64(binary.LittleEndian.Uint16(buf))))
	return nil
}

// push long; decimal string argument
func loadLong(u *Unpickler) error {
	sub, err := u.unsafeReadLine()
	if err != nil {
		return err
	}
	n := len(sub)
	if n == 0 {
		return fmt.Errorf("invalid long data")
	}
	if sub[n-1] == 'L' {
		sub = sub[:n-1]
	}
	i, err := strconv.ParseInt(sub, 10, 64)
	if err == nil {
		u.append(types.NewInt(int64(i)))
		return nil
	}
	if ne, isNe := err.(*strconv.NumError); isNe && ne.Err == strconv.ErrRange {
		bi, ok := new(big.Int).SetString(sub, 10)
		if !ok {
			return fmt.Errorf("invalid long data")
		}
		u.append(types.NewLong(bi))
		return nil
	}
	return err
}

// push long from < 256 bytes
func loadLong1(u *Unpickler) error {
	length, err := u.readOne()
	if err != nil {
		return err
	}
	data, err := u.read(int(length))
	if err != nil {
		return err
	}

	u.append(decodeLong(data))
	return nil
}

// push really big long
func loadLong4(u *Unpickler) error {
	buf, err := u.read(4)
	if err != nil {
		return err
	}
	length := decodeInt32(buf)
	if length < 0 {
		return fmt.Errorf("LONG pickle has negative byte count")
	}
	data, err := u.read(length)
	if err != nil {
		return err
	}

	u.append(decodeLong(data))
	return nil
}

func decodeLong(bytes []byte) types.Object {
	msBitSet := bytes[len(bytes)-1]&0x80 != 0

	if len(bytes) > 8 {
		bi := new(big.Int)
		_ = bytes[len(bytes)-1]
		for i := len(bytes) - 1; i >= 0; i-- {
			bi = bi.Lsh(bi, 8)
			if msBitSet {
				bi = bi.Or(bi, big.NewInt(int64(^bytes[i])))
			} else {
				bi = bi.Or(bi, big.NewInt(int64(bytes[i])))
			}
		}
		if msBitSet {
			bi = bi.Add(bi, big.NewInt(1))
			bi = bi.Neg(bi)
		}
		return types.NewLong(bi)
	}

	var ux, bitMask uint64
	_ = bytes[len(bytes)-1]
	for i := len(bytes) - 1; i >= 0; i-- {
		ux = (ux << 8) | uint64(bytes[i])
		bitMask = (bitMask << 8) | 0xFF
	}
	if msBitSet {
		return types.NewInt(-(int64(^ux&bitMask) + 1))
	}
	return types.NewInt(int64(ux))
}

// push float object; decimal string argument
func loadFloat(u *Unpickler) error {
	line, err := u.unsafeReadLine()
	if err != nil {
		return err
	}
	f, err := strconv.ParseFloat(line, 64)
	if err != nil {
		return err
	}
	u.append(types.NewFloat(f))
	return nil
}

// push float; arg is 8-byte float encoding
func loadBinFloat(u *Unpickler) error {
	buf, err := u.read(8)
	if err != nil {
		return err
	}
	u.append(types.NewFloat(math.Float64frombits(binary.BigEndian.Uint64(buf))))
	return nil
}

// push string; NL-terminated string argument
func loadString(u *Unpickler) error {
	data, err := u.readLineBytes()
	if err != nil {
		return err
	}
	// Strip outermost quotes
	if !isQuotedString(data) {
		return fmt.Errorf("the STRING opcode argument must be quoted")
	}
	data = data[1 : len(data)-1] // remove the quotes
	u.append(u.NewString(data))
	return nil
}

func isQuotedString(b []byte) bool {
	return len(b) >= 2 && b[0] == b[len(b)-1] && (b[0] == '\'' || b[0] == '"')
}

// push string; counted binary string argument
func loadBinString(u *Unpickler) error {
	// Deprecated BINSTRING uses signed 32-bit length
	buf, err := u.read(4)
	if err != nil {
		return err
	}
	length := decodeInt32(buf)
	if length < 0 {
		return fmt.Errorf("BINSTRING pickle has negative byte count")
	}
	data, err := u.read(length)
	if err != nil {
		return err
	}
	u.append(u.NewString(data))
	return nil
}

// push bytes; counted binary string argument
func loadBinBytes(u *Unpickler) error {
	buf, err := u.read(4)
	if err != nil {
		return err
	}
	length := int(binary.LittleEndian.Uint32(buf))
	buf, err = u.read(length)
	if err != nil {
		return err
	}
	u.append(types.ByteArray(buf))
	return nil
}

// push Unicode string; raw-unicode-escaped'd argument
func loadUnicode(u *Unpickler) error {
	line, err := u.readLineBytes()
	if err != nil {
		return err
	}
	u.append(u.NewString(line))
	return nil
}

// push Unicode string; counted UTF-8 string argument
func loadBinUnicode(u *Unpickler) error {
	buf, err := u.read(4)
	if err != nil {
		return err
	}
	length := int(binary.LittleEndian.Uint32(buf))
	buf, err = u.read(length)
	if err != nil {
		return err
	}
	u.append(u.NewString(buf))
	return nil
}

// push very long string
func loadBinUnicode8(u *Unpickler) error {
	buf, err := u.read(8)
	if err != nil {
		return err
	}
	length := binary.LittleEndian.Uint64(buf)
	if length > math.MaxInt64 {
		return fmt.Errorf("BINUNICODE8 exceeds system's maximum size")
	}
	buf, err = u.read(int(length))
	if err != nil {
		return err
	}
	u.append(u.NewString(buf))
	return nil
}

// push very long bytes string
func loadBinBytes8(u *Unpickler) error {
	buf, err := u.read(8)
	if err != nil {
		return err
	}
	length := binary.LittleEndian.Uint64(buf)
	if length > math.MaxInt64 {
		return fmt.Errorf("BINBYTES8 exceeds system's maximum size")
	}
	buf, err = u.read(int(length))
	if err != nil {
		return err
	}
	u.append(types.ByteArray(buf))
	return nil
}

// push bytearray
func loadByteArray8(u *Unpickler) error {
	buf, err := u.read(8)
	if err != nil {
		return err
	}
	length := binary.LittleEndian.Uint64(buf)
	if length > math.MaxInt64 {
		return fmt.Errorf("BYTEARRAY8 exceeds system's maximum size")
	}
	buf, err = u.read(int(length))
	if err != nil {
		return err
	}
	u.append(types.NewByteArray(buf))
	return nil
}

// push next out-of-band buffer
func loadNextBuffer(u *Unpickler) error {
	if u.NextBuffer == nil {
		return fmt.Errorf("pickle stream refers to out-of-band data but NextBuffer was not given")
	}
	buf, err := u.NextBuffer()
	if err != nil {
		return err
	}
	u.append(buf)
	return nil
}

// make top of stack readonly
func loadReadOnlyBuffer(u *Unpickler) error {
	if u.MakeReadOnly == nil {
		return nil
	}
	buf, err := u.stackPop()
	if err != nil {
		return err
	}
	buf, err = u.MakeReadOnly(buf)
	if err != nil {
		return err
	}
	u.append(buf)
	return nil
}

// push string; counted binary string argument < 256 bytes
func loadShortBinString(u *Unpickler) error {
	length, err := u.readOne()
	if err != nil {
		return err
	}
	data, err := u.read(int(length))
	if err != nil {
		return err
	}
	u.append(u.NewString(data))
	return nil
}

// push bytes; counted binary string argument < 256 bytes
func loadShortBinBytes(u *Unpickler) error {
	length, err := u.readOne()
	if err != nil {
		return err
	}
	buf, err := u.read(int(length))
	if err != nil {
		return err
	}
	u.append(types.ByteArray(buf))
	return nil
}

// push short string; UTF-8 length < 256 bytes
func loadShortBinUnicode(u *Unpickler) error {
	length, err := u.readOne()
	if err != nil {
		return err
	}
	buf, err := u.read(int(length))
	if err != nil {
		return err
	}
	u.append(u.NewString(buf))
	return nil
}

// build tuple from topmost stack items
func loadTuple(u *Unpickler) error {
	items, err := u.popMark()
	if err != nil {
		return err
	}
	u.append(types.NewTupleFromSlice(items))
	return nil
}

// push empty tuple
func loadEmptyTuple(u *Unpickler) error {
	u.append(types.NewTupleFromSlice([]types.Object{}))
	return nil
}

// build 1-tuple from stack top
func loadTuple1(u *Unpickler) error {
	value, err := u.stackPop()
	if err != nil {
		return err
	}
	u.append(types.NewTupleFromSlice([]types.Object{value}))
	return nil
}

// build 2-tuple from two topmost stack items
func loadTuple2(u *Unpickler) error {
	second, err := u.stackPop()
	if err != nil {
		return err
	}
	first, err := u.stackPop()
	if err != nil {
		return err
	}
	u.append(types.NewTupleFromSlice([]types.Object{first, second}))
	return nil
}

// build 3-tuple from three topmost stack items
func loadTuple3(u *Unpickler) error {
	third, err := u.stackPop()
	if err != nil {
		return err
	}
	second, err := u.stackPop()
	if err != nil {
		return err
	}
	first, err := u.stackPop()
	if err != nil {
		return err
	}
	u.append(types.NewTupleFromSlice([]types.Object{first, second, third}))
	return nil
}

// push empty list
func loadEmptyList(u *Unpickler) error {
	u.append(types.NewList(u.alloc_dram()))
	return nil
}

// push empty dict
func loadEmptyDict(u *Unpickler) error {
	u.append(types.NewDict(0, u.alloc_dram()))
	return nil
}

// push empty set on the stack
func loadEmptySet(u *Unpickler) error {
	u.append(types.NewSet(u.alloc_dram()))
	return nil
}

// build frozenset from topmost stack items
func loadFrozenSet(u *Unpickler) error {
	items, err := u.popMark()
	if err != nil {
		return err
	}
	u.append(types.NewFrozenSetFromSlice(items))
	return nil
}

// build list from topmost stack items
func loadList(u *Unpickler) error {
	items, err := u.popMark()
	if err != nil {
		return err
	}
	u.append(types.NewListFromSlice(items, u.alloc_dram()))
	return nil
}

// build a dict from stack items
func loadDict(u *Unpickler) error {
	items, err := u.popMark()
	if err != nil {
		return err
	}
	itemsLen := len(items)
	d := types.NewDict(itemsLen/2, u.alloc_dram())
	for i := 0; i < itemsLen-1; i += 2 {
		d.Set(items[i], items[i+1])
	}
	u.append(d)
	return nil
}

// build & push class instance
func loadInst(u *Unpickler) error {
	// NOTE: we want readLineBytes() which returns a []byte, and then cast (copy) to string. If we used readLine() which returns an unsafe string, we could be passing a mutable (after the outer call returns) string to a user define callback function u.FindClass
	module, err := u.readLineBytes()
	if err != nil {
		return err
	}

	name, err := u.readLineBytes()
	if err != nil {
		return err
	}

	class, err := u.findClass(string(module), string(name))
	if err != nil {
		return err
	}

	args, err := u.popMark()
	if err != nil {
		return err
	}

	return u.instantiate(class, args)
}

// build & push class instance
func loadObj(u *Unpickler) error {
	// Stack is ... markobject classobject arg1 arg2 ...
	args, err := u.popMark()
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return fmt.Errorf("OBJ class missing")
	}
	class := args[0]
	args = args[1:]
	return u.instantiate(class, args)
}

func (u *Unpickler) instantiate(class types.Object, args []types.Object) error {
	var err error
	var value types.Object
	switch ct := class.(type) {
	case types.Callable:
		value, err = ct.Call(args...)
	case types.PyNewable:
		value, err = ct.PyNew(args...)
	default:
		return fmt.Errorf("cannot instantiate %#v", class)
	}

	if err != nil {
		return err
	}
	u.append(value)
	return nil
}

// build object by applying cls.__new__ to argtuple
func loadNewObj(u *Unpickler) error {
	args, err := u.stackPop()
	if err != nil {
		return err
	}
	argsTuple, argsOk := args.(types.Tuple)
	if !argsOk {
		return fmt.Errorf("NEWOBJ args must be *Tuple")
	}

	rawClass, err := u.stackPop()
	if err != nil {
		return err
	}
	class, classOk := rawClass.(types.PyNewable)
	if !classOk {
		return fmt.Errorf("NEWOBJ requires a PyNewable object: %#v", rawClass)
	}

	result, err := class.PyNew(argsTuple...)
	if err != nil {
		return err
	}
	u.append(result)
	return nil
}

// like NEWOBJ but work with keyword only arguments
func loadNewObjEx(u *Unpickler) error {
	kwargs, err := u.stackPop()
	if err != nil {
		return err
	}

	args, err := u.stackPop()
	if err != nil {
		return err
	}
	argsTuple, argsOk := args.(types.Tuple)
	if !argsOk {
		return fmt.Errorf("NEWOBJ_EX args must be *Tuple")
	}

	rawClass, err := u.stackPop()
	if err != nil {
		return err
	}
	class, classOk := rawClass.(types.PyNewable)
	if !classOk {
		return fmt.Errorf("NEWOBJ_EX requires a PyNewable object")
	}

	allArgs := []types.Object(argsTuple)
	allArgs = append(allArgs, kwargs)

	result, err := class.PyNew(allArgs...)
	if err != nil {
		return err
	}
	u.append(result)
	return nil
}

// push self.find_class(modname, name); 2 string args
func loadGlobal(u *Unpickler) error {
	// NOTE: we want readLineBytes() which returns a []byte, and then cast (copy) to string. If we used readLine() which returns an unsafe string, we could be passing a mutable (after the outer call returns) string to a user define callback function u.FindClass
	module, err := u.readLineBytes()
	if err != nil {
		return err
	}

	name, err := u.readLineBytes()
	if err != nil {
		return err
	}

	class, err := u.findClass(string(module), string(name))
	if err != nil {
		return err
	}
	u.append(class)
	return nil
}

// same as GLOBAL but using names on the stacks
func loadStackGlobal(u *Unpickler) error {
	rawName, err := u.stackPop()
	if err != nil {
		return err
	}
	name, nameOk := rawName.(types.String)
	if !nameOk {
		return fmt.Errorf("STACK_GLOBAL requires str name: %#v", rawName)
	}

	rawModule, err := u.stackPop()
	if err != nil {
		return err
	}
	module, moduleOk := rawModule.(types.String)
	if !moduleOk {
		return fmt.Errorf("STACK_GLOBAL requires str module: %#v", rawModule)
	}

	class, err := u.findClass(module.String(), name.String())
	if err != nil {
		return err
	}
	u.append(class)
	return nil
}

// push object from extension registry; 1-byte index
func opExt1(u *Unpickler) error {
	if u.GetExtension == nil {
		return fmt.Errorf("unsupported extension code encountered")
	}
	i, err := u.readOne()
	if err != nil {
		return err
	}
	obj, err := u.GetExtension(int(i))
	if err != nil {
		return err
	}
	u.append(obj)
	return nil
}

// ditto, but 2-byte index
func opExt2(u *Unpickler) error {
	if u.GetExtension == nil {
		return fmt.Errorf("unsupported extension code encountered")
	}
	buf, err := u.read(2)
	if err != nil {
		return err
	}
	code := int(binary.LittleEndian.Uint16(buf))
	obj, err := u.GetExtension(code)
	if err != nil {
		return err
	}
	u.append(obj)
	return nil
}

// ditto, but 4-byte index
func opExt4(u *Unpickler) error {
	if u.GetExtension == nil {
		return fmt.Errorf("unsupported extension code encountered")
	}
	buf, err := u.read(4)
	if err != nil {
		return err
	}
	code := int(binary.LittleEndian.Uint32(buf))
	obj, err := u.GetExtension(code)
	if err != nil {
		return err
	}
	u.append(obj)
	return nil
}

// apply callable to argtuple, both on stack
func loadReduce(u *Unpickler) error {
	args, err := u.stackPop()
	if err != nil {
		return err
	}
	argsTuple, argsOk := args.(types.Tuple)
	if !argsOk {
		return fmt.Errorf("REDUCE args must be *Tuple")
	}

	function, err := u.stackPop()
	if err != nil {
		return err
	}
	callable, callableOk := function.(types.Callable)
	if !callableOk {
		return fmt.Errorf("REDUCE requires a Callable object: %#v", function)
	}

	result, err := callable.Call(argsTuple...)
	if err != nil {
		return err
	}
	u.append(result)
	return nil
}

// discard topmost stack item
func loadPop(u *Unpickler) error {
	if len(u.stack) == 0 {
		_, err := u.popMark()
		return err
	}
	u.stack = u.stack[:len(u.stack)-1]
	return nil
}

// discard stack top through topmost markobject
func loadPopMark(u *Unpickler) error {
	_, err := u.popMark()
	return err
}

// duplicate top stack item
func loadDup(u *Unpickler) error {
	item, err := u.stackLast()
	if err != nil {
		return err
	}
	u.append(item)
	return nil
}

// push item from memo on stack; index is string arg
func loadGet(u *Unpickler) error {
	line, err := u.unsafeReadLine()
	if err != nil {
		return err
	}
	i, err := strconv.ParseUint(line, 10, 32)
	if err != nil {
		return err
	}
	u.append(u.memo[uint32(i)])
	return nil
}

// push item from memo on stack; index is 1-byte arg
func loadBinGet(u *Unpickler) error {
	i, err := u.readOne()
	if err != nil {
		return err
	}
	u.append(u.memo[uint32(i)])
	return nil
}

// push item from memo on stack; index is 4-byte arg
func loadLongBinGet(u *Unpickler) error {
	buf, err := u.read(4)
	if err != nil {
		return err
	}
	i := binary.LittleEndian.Uint32(buf)
	u.append(u.memo[i])
	return nil
}

// store stack top in memo; index is string arg
func loadPut(u *Unpickler) error {
	line, err := u.unsafeReadLine()
	if err != nil {
		return err
	}
	i, err := strconv.ParseUint(line, 10, 32)
	if err != nil {
		return err
	}
	u.memo[uint32(i)], err = u.stackLast()
	return err
}

// store stack top in memo; index is 1-byte arg
func loadBinPut(u *Unpickler) error {
	i, err := u.readOne()
	if err != nil {
		return err
	}
	u.memo[uint32(i)], err = u.stackLast()
	return err
}

// store stack top in memo; index is 4-byte arg
func loadLongBinPut(u *Unpickler) error {
	buf, err := u.read(4)
	if err != nil {
		return err
	}
	i := binary.LittleEndian.Uint32(buf)
	u.memo[i], err = u.stackLast()
	return err
}

// store top of the stack in memo
func loadMemoize(u *Unpickler) error {
	value, err := u.stackLast()
	if err != nil {
		return err
	}
	u.memo[uint32(len(u.memo))] = value
	return nil
}

// append stack top to list below it
func loadAppend(u *Unpickler) error {
	value, err := u.stackPop()
	if err != nil {
		return err
	}
	obj, err := u.stackLast()
	if err != nil {
		return err
	}
	list, listOk := obj.(types.ListAppender)
	if !listOk {
		return fmt.Errorf("APPEND requires ListAppender")
	}
	list.Append(value)
	return nil
}

// extend list on stack by topmost stack slice
func loadAppends(u *Unpickler) error {
	items, err := u.popMark()
	if err != nil {
		return err
	}
	obj, err := u.stackLast()
	if err != nil {
		return err
	}
	list, listOk := obj.(types.ListAppender)
	if !listOk {
		return fmt.Errorf("APPEND requires List")
	}
	list.AppendMany(items)
	return nil
}

// add key+value pair to dict
func loadSetItem(u *Unpickler) error {
	value, err := u.stackPop()
	if err != nil {
		return err
	}
	key, err := u.stackPop()
	if err != nil {
		return err
	}
	obj, err := u.stackLast()
	if err != nil {
		return err
	}
	dict, dictOk := obj.(types.DictSetter)
	if !dictOk {
		return fmt.Errorf("SETITEM requires DictSetter")
	}
	dict.Set(key, value)
	return nil
}

// modify dict by adding topmost key+value pairs
func loadSetItems(u *Unpickler) error {
	items, err := u.popMark()
	if err != nil {
		return err
	}
	obj, err := u.stackLast()
	if err != nil {
		return err
	}
	dict, dictOk := obj.(types.DictSetter)
	if !dictOk {
		return fmt.Errorf("SETITEMS requires DictSetter")
	}
	dict.SetMany(items)
	return nil
}

// modify set by adding topmost stack items
func loadAddItems(u *Unpickler) error {
	items, err := u.popMark()
	if err != nil {
		return err
	}
	obj, err := u.stackLast()
	if err != nil {
		return err
	}
	set, setOk := obj.(types.SetAdder)
	if !setOk {
		return fmt.Errorf("ADDITEMS requires SetAdder")
	}
	set.AddMany(items)
	return nil
}

// call __setstate__ or __dict__.update()
func loadBuild(u *Unpickler) error {
	state, err := u.stackPop()
	if err != nil {
		return err
	}
	inst, err := u.stackLast()
	if err != nil {
		return err
	}
	if obj, ok := inst.(types.PyStateSettable); ok {
		return obj.PySetState(state)
	}

	var slotState types.Object
	if tuple, ok := state.(types.Tuple); ok && tuple.Len() == 2 {
		state = tuple.Get(0)
		slotState = tuple.Get(1)
	}

	if stateDict, ok := state.(*types.Dict); ok {
		_ = stateDict
		panic("stateDict not implemented")
		/*
			instPds, instPdsOk := inst.(types.PyDictSettable)
			if !instPdsOk {
				return fmt.Errorf("BUILD requires a PyDictSettable instance: %#v", inst)
			}
			for _, entry := range *stateDict {
				err := instPds.PyDictSet(entry.Key, entry.Value)
				if err != nil {
					return err
				}
			}
		*/
	}

	if slotStateDict, ok := slotState.(*types.Dict); ok {
		_ = slotStateDict
		panic("slotStateDict not implemented")
		/*
			/*
				instSa, instOk := inst.(types.PyAttrSettable)
				if !instOk {
					return fmt.Errorf(
						"BUILD requires a PyAttrSettable instance: %#v", inst)
				}
				for _, entry := range *slotStateDict {
					sk, keyOk := entry.Key.(types.String)
					if !keyOk {
						return fmt.Errorf("BUILD requires string slot state keys")
					}
					err := instSa.PySetAttr(sk.String(), entry.Value)
					if err != nil {
						return err
					}
				}
		*/
	}

	return nil
}

// push special markobject on stack
func loadMark(u *Unpickler) error {
	u.metaStack = append(u.metaStack, u.stack)
	if len(u.ram) < 16 {
		u.ram = make([]types.Object, 16*128)
		u.ram = u.ram[:cap(u.ram)]
	}
	u.stack, u.ram = u.ram[0:0:16], u.ram[16:]
	return nil
}

// every pickle ends with STOP
func loadStop(u *Unpickler) error {
	value, err := u.stackPop()
	if err != nil {
		return err
	}
	return pickleStop{value: value}
}

func decodeInt32(b []byte) int {
	ux := binary.LittleEndian.Uint32(b)
	x := int(ux)
	if b[3]&0x80 != 0 {
		x = -(int(^ux) + 1)
	}
	return x
}
