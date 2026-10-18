package world

const pairTomb = ^uint64(0)

type pairMap struct {
	keys   []uint64
	vals   []int32
	spareK []uint64
	spareV []int32
	live   int
	used   int
	shift  uint
}

func shiftFor(size int) uint {
	s := uint(64)
	for n := size; n > 1; n >>= 1 {
		s--
	}
	return s
}

func newPairMap(capacity int) pairMap {
	size := 16
	for size*3 < capacity*4 {
		size <<= 1
	}
	return pairMap{keys: make([]uint64, size), vals: make([]int32, size), shift: shiftFor(size)}
}

func pairKey(a, b int32) uint64 {
	if a > b {
		a, b = b, a
	}
	return uint64(uint32(a)+1)<<32 | uint64(uint32(b)+1)
}

func (m *pairMap) slot(k uint64) int {
	return int((k * 0x9E3779B97F4A7C15) >> m.shift)
}

func (m *pairMap) get(k uint64) (int32, bool) {
	mask := len(m.keys) - 1
	for i := m.slot(k); ; i = (i + 1) & mask {
		switch m.keys[i] {
		case k:
			return m.vals[i], true
		case 0:
			return 0, false
		}
	}
}

func (m *pairMap) put(k uint64, v int32) {
	if (m.used+1)*4 >= len(m.keys)*3 {
		m.rehash()
	}
	mask := len(m.keys) - 1
	tomb := -1
	for i := m.slot(k); ; i = (i + 1) & mask {
		switch m.keys[i] {
		case k:
			m.vals[i] = v
			return
		case pairTomb:
			if tomb < 0 {
				tomb = i
			}
		case 0:
			if tomb >= 0 {
				i = tomb
			} else {
				m.used++
			}
			m.keys[i], m.vals[i] = k, v
			m.live++
			return
		}
	}
}

func (m *pairMap) del(k uint64) bool {
	mask := len(m.keys) - 1
	for i := m.slot(k); ; i = (i + 1) & mask {
		switch m.keys[i] {
		case k:
			m.keys[i] = pairTomb
			m.live--
			return true
		case 0:
			return false
		}
	}
}

func (m *pairMap) rehash() {
	oldK, oldV := m.keys, m.vals
	size := len(oldK)
	if (m.live+1)*2 >= size {
		size <<= 1
	}
	if size == len(oldK) && len(m.spareK) == size {
		m.keys, m.vals = m.spareK, m.spareV
		clear(m.keys)
	} else {
		m.keys, m.vals = make([]uint64, size), make([]int32, size)
	}
	m.spareK, m.spareV = oldK, oldV
	m.live, m.used, m.shift = 0, 0, shiftFor(size)
	for i, k := range oldK {
		if k != 0 && k != pairTomb {
			m.put(k, oldV[i])
		}
	}
}

func (m *pairMap) cloneInto(dst *pairMap) {
	dst.keys = append(dst.keys[:0], m.keys...)
	dst.vals = append(dst.vals[:0], m.vals...)
	dst.live, dst.used, dst.shift = m.live, m.used, m.shift
}
