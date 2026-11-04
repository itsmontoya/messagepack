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
