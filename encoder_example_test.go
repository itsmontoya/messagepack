package messagepack_test

import (
	"bytes"
	"fmt"

	"github.com/itsmontoya/messagepack"
)

func ExampleEncoder() {
	var (
		output bytes.Buffer
		e      = messagepack.NewEncoder(&output)
		err    error
	)

	if err = e.Encode(exampleMessage{ID: 42}); err != nil {
		panic(err)
	}

	if err = e.Close(); err != nil {
		panic(err)
	}

	fmt.Printf("%x\n", output.Bytes())
	// Output: 81a269642a
}

func ExampleEncoder_reflection() {
	var (
		output  bytes.Buffer
		value   = struct{ ID int }{ID: 42}
		decoded struct{ ID int }
		err     error
	)

	if err = messagepack.NewEncoder(&output).Encode(value); err != nil {
		panic(err)
	}

	if err = messagepack.NewDecoder(&output).Decode(&decoded); err != nil {
		panic(err)
	}

	fmt.Println(decoded.ID)
	// Output: 42
}
