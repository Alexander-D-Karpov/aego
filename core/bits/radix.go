package bits

const radixBits = 8

const radixBuckets = 1 << radixBits

func RadixSort64(keys, scratch []uint64) []uint64 {
	if len(keys) < 2 {
		return keys
	}
	if len(scratch) < len(keys) {
		scratch = make([]uint64, len(keys))
	}
	src := keys
	dst := scratch[:len(keys)]
	var counts [radixBuckets]int
	for shift := 0; shift < 64; shift += radixBits {
		for i := range counts {
			counts[i] = 0
		}
		for _, k := range src {
			counts[(k>>uint(shift))&(radixBuckets-1)]++
		}
		if counts[(src[0]>>uint(shift))&(radixBuckets-1)] == len(src) {
			continue
		}
		sum := 0
		for i := range counts {
			c := counts[i]
			counts[i] = sum
			sum += c
		}
		for _, k := range src {
			b := (k >> uint(shift)) & (radixBuckets - 1)
			dst[counts[b]] = k
			counts[b]++
		}
		src, dst = dst, src
	}
	return src
}
