package messagepack

const (
	defaultMaxPayloadBytes = 16777216
	defaultMaxSkipDepth    = 64
	maximumSkipDepth       = 256
	defaultMaxSkipValues   = 1048576
	defaultMaxSkipBytes    = 67108864
)

// ReaderLimits bounds owned payload results and the work of each Skip call.
// A zero field selects its finite default; no field can disable its limit.
type ReaderLimits struct {
	// MaxPayloadBytes caps each owned string/binary result; the default is 16 MiB.
	MaxPayloadBytes uint64
	// MaxSkipDepth caps nested container levels; the default is 64, maximum 256.
	MaxSkipDepth uint32
	// MaxSkipValues caps values, including containers and map keys; the default is 1048576.
	MaxSkipValues uint64
	// MaxSkipBytes caps consumed wire bytes per Skip; the default is 64 MiB.
	MaxSkipBytes uint64
}

func (l ReaderLimits) normalized() (out ReaderLimits, err error) {
	if l.MaxPayloadBytes > 4294967295 || l.MaxSkipDepth > maximumSkipDepth {
		return ReaderLimits{}, ErrInvalidArgument
	}
	out = l
	if out.MaxPayloadBytes == 0 {
		out.MaxPayloadBytes = defaultMaxPayloadBytes
	}
	if out.MaxSkipDepth == 0 {
		out.MaxSkipDepth = defaultMaxSkipDepth
	}
	if out.MaxSkipValues == 0 {
		out.MaxSkipValues = defaultMaxSkipValues
	}
	if out.MaxSkipBytes == 0 {
		out.MaxSkipBytes = defaultMaxSkipBytes
	}
	return out, nil
}
