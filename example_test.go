package messagepack

import "fmt"

func Example() {
	var (
		message = applicationMessage{ID: 7, Name: "Ada", Active: true, Score: 1.5,
			Payload: []byte{1, 2}, Samples: [2]int64{-1, 2}}
		dst     = make([]byte, 0, 128)
		backing = &dst[:cap(dst)][0]
		decoded applicationMessage
		err     error
	)
	dst, err = message.appendTo(dst)
	if err != nil {
		fmt.Println("encode:", err)
		return
	}
	// The fixed-schema decoder validates every field name and both container counts.
	// Requiring full consumption rejects unwanted trailing values.
	decoded, err = readCompleteApplicationMessage(dst)
	if err != nil {
		fmt.Println("decode:", err)
		return
	}
	fmt.Printf("%d %s %t %.1f %x %v\n", decoded.ID, decoded.Name, decoded.Active,
		decoded.Score, decoded.Payload, decoded.Samples)
	// Copy the borrowed payload before reusing its encoded source storage.
	ownedPayload := append([]byte(nil), decoded.Payload...)
	dst, err = message.appendTo(dst[:0])
	if err != nil {
		fmt.Println("reuse:", err)
		return
	}
	fmt.Printf("reused: %t, copied payload: %x\n", &dst[0] == backing, ownedPayload)
	// Output:
	// 7 Ada true 1.5 0102 [-1 2]
	// reused: true, copied payload: 0102
}

func Example_concatenatedMessages() {
	var (
		message = applicationMessage{ID: 7, Name: "Ada", Active: true, Score: 1.5,
			Payload: []byte{1, 2}, Samples: [2]int64{-1, 2}}
		wire    []byte
		decoded applicationMessage
		n       int
		err     error
	)
	wire, err = message.appendTo(wire)
	if err != nil {
		fmt.Println("first encode:", err)
		return
	}
	message.ID = 8
	wire, err = message.appendTo(wire)
	if err != nil {
		fmt.Println("second encode:", err)
		return
	}
	for index := 0; index < 2; index++ {
		decoded, n, err = readApplicationMessage(wire)
		if err != nil {
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
