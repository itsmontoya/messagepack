package messagepack_test

import (
	"bytes"
	"fmt"

	"github.com/itsmontoya/messagepack"
)

func ExampleDecoder() {
	var (
		input   = bytes.NewReader([]byte{0x81, 0xa2, 'i', 'd', 42})
		d       = messagepack.NewDecoder(input)
		message exampleMessage
		err     error
	)
	if err = d.Decode(&message); err != nil {
		panic(err)
	}
	if err = d.Close(); err != nil {
		panic(err)
	}
	fmt.Println(message.ID)
	// Output: 42
}
