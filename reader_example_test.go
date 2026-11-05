package messagepack_test

import (
	"bytes"
	"fmt"

	"github.com/itsmontoya/messagepack"
)

func ExampleReader_DecodeInt64s() {
	var (
		input  = bytes.NewReader([]byte{0x93, 0xff, 0xcc, 0x80, 0xd1, 0xff, 0x7f, 0xc3})
		r      = messagepack.NewReader(input)
		values [3]int64
		count  uint32
		n      int
		next   bool
		err    error
	)

	if count, err = r.DecodeArrayHeader(); err != nil {
		panic(err)
	}

	if count != uint32(len(values)) {
		panic("unexpected array length")
	}

	if n, err = r.DecodeInt64s(values[:]); err != nil {
		panic(err)
	}

	if next, err = r.DecodeBool(); err != nil {
		panic(err)
	}

	fmt.Println(n, values)
	fmt.Println(next)
	// Output:
	// 3 [-1 128 -129]
	// true
}
