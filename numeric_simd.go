//go:build goexperiment.simd && (arm64 || amd64)

package messagepack

import (
	"io"
	"simd/archsimd"
)

const numericSIMDEnabled = true

// loadNumericPair removes two MessagePack tags and reverses both big-endian bodies.
// Overlapping loads stay inside the eighteen bytes belonging to these two values.
func loadNumericPair(wire []byte) (values archsimd.Uint64x2) {
	var (
		lowIndices  = [16]uint8{8, 7, 6, 5, 4, 3, 2, 1, 128, 128, 128, 128, 128, 128, 128, 128}
		highIndices = [16]uint8{128, 128, 128, 128, 128, 128, 128, 128, 15, 14, 13, 12, 11, 10, 9, 8}
		low         = shuffleNumericBytes(archsimd.LoadUint8x16(wire[:16]), archsimd.LoadUint8x16Array(&lowIndices))
		high        = shuffleNumericBytes(archsimd.LoadUint8x16(wire[2:18]), archsimd.LoadUint8x16Array(&highIndices))
	)

	return low.Or(high).ReshapeToUint64s()
}

func loadPositiveFixints(wire []byte) (values archsimd.Uint8x16, valid bool) {
	values = archsimd.LoadUint8x16(wire[:16])
	var highBits = values.And(archsimd.BroadcastUint8x16(0x80)).ReshapeToUint64s()
	return values, highBits.GetElem(0)|highBits.GetElem(1) == 0
}

func decodePositiveInt64s(wire []byte, dst []int64) (n int) {
	var (
		values, valid = loadPositiveFixints(wire)
		indices       = [16]uint8{0, 128, 128, 128, 128, 128, 128, 128, 1, 128, 128, 128, 128, 128, 128, 128}
		selection     = archsimd.LoadUint8x16Array(&indices)
		step          = archsimd.BroadcastUint8x16(2)
	)

	if !valid {
		return 0
	}

	dst = dst[:16]
	for n = 0; n <= len(dst)-2; n += 2 {
		shuffleNumericBytes(values, selection).ReshapeToUint64s().BitsToInt64().Store(dst[n:])
		selection = selection.Add(step)
	}
	return n
}

func decodePositiveUint64s(wire []byte, dst []uint64) (n int) {
	var (
		values, valid = loadPositiveFixints(wire)
		indices       = [16]uint8{0, 128, 128, 128, 128, 128, 128, 128, 1, 128, 128, 128, 128, 128, 128, 128}
		selection     = archsimd.LoadUint8x16Array(&indices)
		step          = archsimd.BroadcastUint8x16(2)
	)

	if !valid {
		return 0
	}

	dst = dst[:16]
	for n = 0; n <= len(dst)-2; n += 2 {
		shuffleNumericBytes(values, selection).ReshapeToUint64s().Store(dst[n:])
		selection = selection.Add(step)
	}
	return n
}

func decodeInt64sSIMD(source io.Reader, dst []int64) (n int, err error) {
	if !numericSIMDSupported() {
		return 0, nil
	}

	var input = newNumericBatchInput(source)
	for len(dst)-n >= 2 {
		var wire []byte
		if wire, err = input.peek(min(len(dst)-n, numericBatchElements) * 9); err != nil {
			return n, err
		}

		var (
			completed int
			consumed  int
		)

		for len(wire) >= 16 && len(dst)-n-completed >= 2 {
			if wire[0] <= 0x7f && len(dst)-n-completed >= 16 && decodePositiveInt64s(wire, dst[n+completed:]) == 16 {
				completed += 16
				consumed += 16
				wire = wire[16:]
				continue
			}

			if len(wire) < 18 {
				break
			}

			if !(wire[0] == 0xd3 || wire[0] == 0xcf && wire[1] < 0x80) ||
				!(wire[9] == 0xd3 || wire[9] == 0xcf && wire[10] < 0x80) {
				break
			}

			loadNumericPair(wire).BitsToInt64().Store(dst[n+completed:])
			completed += 2
			consumed += 18
			wire = wire[18:]
		}
		if completed == 0 {
			return n, nil
		}

		n += completed
		if err = input.discard(consumed); err != nil {
			return n, err
		}
	}
	return n, nil
}

func decodeUint64sSIMD(source io.Reader, dst []uint64) (n int, err error) {
	if !numericSIMDSupported() {
		return 0, nil
	}

	var input = newNumericBatchInput(source)
	for len(dst)-n >= 2 {
		var wire []byte
		if wire, err = input.peek(min(len(dst)-n, numericBatchElements) * 9); err != nil {
			return n, err
		}

		var (
			completed int
			consumed  int
		)

		for len(wire) >= 16 && len(dst)-n-completed >= 2 {
			if wire[0] <= 0x7f && len(dst)-n-completed >= 16 && decodePositiveUint64s(wire, dst[n+completed:]) == 16 {
				completed += 16
				consumed += 16
				wire = wire[16:]
				continue
			}

			if len(wire) < 18 {
				break
			}

			if !(wire[0] == 0xcf || wire[0] == 0xd3 && wire[1] < 0x80) ||
				!(wire[9] == 0xcf || wire[9] == 0xd3 && wire[10] < 0x80) {
				break
			}

			loadNumericPair(wire).Store(dst[n+completed:])
			completed += 2
			consumed += 18
			wire = wire[18:]
		}
		if completed == 0 {
			return n, nil
		}

		n += completed
		if err = input.discard(consumed); err != nil {
			return n, err
		}
	}
	return n, nil
}

func decodeFloat64sSIMD(source io.Reader, dst []float64) (n int, err error) {
	if !numericSIMDSupported() {
		return 0, nil
	}

	var input = newNumericBatchInput(source)
	for len(dst)-n >= 2 {
		var wire []byte
		if wire, err = input.peek(min(len(dst)-n, numericBatchElements) * 9); err != nil {
			return n, err
		}

		var completed int
		for len(wire) >= 18 && len(dst)-n-completed >= 2 {
			if wire[0] != 0xcb || wire[9] != 0xcb {
				break
			}

			loadNumericPair(wire).BitsToFloat64().Store(dst[n+completed:])
			completed += 2
			wire = wire[18:]
		}
		if completed == 0 {
			return n, nil
		}

		n += completed
		if err = input.discard(completed * 9); err != nil {
			return n, err
		}
	}
	return n, nil
}
