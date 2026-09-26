package schema

const (
	fnvOffset uint64 = 14695981039346656037
	fnvPrime  uint64 = 1099511628211
)

func hashBytes(h uint64, s string) uint64 {
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= fnvPrime
	}
	return h
}

func HashName(full string) TypeID {
	return TypeID(hashBytes(fnvOffset, full))
}

func HashField(t TypeID, field string) FieldID {
	h := uint64(t)
	h ^= 0x5bf03635
	h *= fnvPrime
	return FieldID(hashBytes(h, field))
}
