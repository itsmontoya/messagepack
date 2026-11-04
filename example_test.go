package messagepack

import (
	"bytes"
	"fmt"
)

func Example() {
	var (
		message = applicationMessage{ID: 7, Name: "Ada", Active: true, Score: 1.5,
			Payload: []byte{1, 2}, Samples: [2]int64{-1, 2}}
		output  = bytes.NewBuffer(make([]byte, 0, 128))
		e       = NewEncoder(output)
		decoded applicationMessage
		err     error
	)

	if err = e.Encode(message); err != nil {
		fmt.Println("encode:", err)
		return
	}

	backing := &output.Bytes()[0]
	// The fixed-schema decoder validates every field name and both container counts.
	// Requiring full consumption rejects unwanted trailing values.
	if decoded, err = readCompleteApplicationMessage(output.Bytes()); err != nil {
		fmt.Println("decode:", err)
		return
	}

	fmt.Printf("%d %s %t %.1f %x %v\n", decoded.ID, decoded.Name, decoded.Active,
		decoded.Score, decoded.Payload, decoded.Samples)
	// Owned payloads remain independent when the encoded source storage is reused.
	ownedPayload := append([]byte(nil), decoded.Payload...)
	message.Payload = ownedPayload
	output.Reset()
	if err = e.Encode(message); err != nil {
		fmt.Println("reuse:", err)
		return
	}

	fmt.Printf("reused: %t, copied payload: %x\n", &output.Bytes()[0] == backing, ownedPayload)
	// Output:
	// 7 Ada true 1.5 0102 [-1 2]
	// reused: true, copied payload: 0102
}

func Example_concatenatedMessages() {
	var (
		message = applicationMessage{ID: 7, Name: "Ada", Active: true, Score: 1.5,
			Payload: []byte{1, 2}, Samples: [2]int64{-1, 2}}
		output  bytes.Buffer
		e       = NewEncoder(&output)
		decoded applicationMessage
		n       int
		err     error
	)

	if err = e.Encode(message); err != nil {
		fmt.Println("first encode:", err)
		return
	}

	message.ID = 8
	if err = e.Encode(message); err != nil {
		fmt.Println("second encode:", err)
		return
	}

	var wire = output.Bytes()
	for index := 0; index < 2; index++ {
		if decoded, n, err = readApplicationMessage(wire); err != nil {
			fmt.Println("decode:", err)
			return
		}

		fmt.Println("id:", decoded.ID)
		wire = wire[n:]
	}
	if len(wire) != 0 {
		fmt.Println("decode:", errApplicationTrailing)
		return
	}

	fmt.Println("trailing bytes:", len(wire))
	// Output:
	// id: 7
	// id: 8
	// trailing bytes: 0
}
