package core

type ID uint64

const (
	fnvOffset uint64 = 14695981039346656037
	fnvPrime  uint64 = 1099511628211
)

func IDOf(s string) ID {
	h := fnvOffset
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= fnvPrime
	}
	return ID(h)
}

func IDSeq(parent ID, i int) ID {
	h := uint64(parent)
	v := uint64(uint32(i))
	for shift := 0; shift < 32; shift += 8 {
		h ^= (v >> uint(shift)) & 0xff
		h *= fnvPrime
	}
	return ID(h)
}
