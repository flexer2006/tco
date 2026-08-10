package pipeline

const (
	annBits              = 32
	annRecentExactCap    = similarityTileSize
	defaultANNActivateAt = 512
	splitmixGamma        = 0x9e3779b97f4a7c15
	splitmixMul1         = 0xbf58476d1ce4e5b9
	splitmixMul2         = 0x94d049bb133111eb
	splitmixFinalShift   = 31
	splitmixMixShift1    = 30
	splitmixMixShift2    = 27
)

//nolint:gochecknoglobals // overridable activation threshold for tests
var annActivateAt = defaultANNActivateAt

type projectionIndex struct {
	planes  [][]float32
	sigs    []uint32
	buckets map[uint32][]int
	dim     int
}

func newProjectionIndex(capacity, dim int) *projectionIndex {
	if dim <= 0 {
		return new(projectionIndex{
			planes:  nil,
			buckets: make(map[uint32][]int),
			sigs:    nil,
			dim:     0,
		})
	}

	planes := make([][]float32, annBits)
	for plane := range annBits {
		row := make([]float32, dim)
		state := splitmixSeed(uint64(plane+1), uint64(dim))

		for dimIdx := range dim {
			state = splitmix64(state)
			if state&1 == 0 {
				row[dimIdx] = -1
			} else {
				row[dimIdx] = 1
			}
		}

		planes[plane] = row
	}

	return new(projectionIndex{
		planes:  planes,
		buckets: make(map[uint32][]int, capacity),
		sigs:    make([]uint32, 0, capacity),
		dim:     dim,
	})
}

func (idx *projectionIndex) signature(row []float32) uint32 {
	if idx == nil || len(idx.planes) == 0 || len(row) == 0 {
		return 0
	}

	var sig uint32

	n := min(len(row), idx.dim)
	for plane, hyperplane := range idx.planes {
		var dot float64
		for dimIdx := range n {
			dot += float64(row[dimIdx]) * float64(hyperplane[dimIdx])
		}

		if dot >= 0 {
			sig |= 1 << uint(plane)
		}
	}

	return sig
}

func (idx *projectionIndex) add(row []float32) int {
	if idx == nil {
		return -1
	}

	i := len(idx.sigs)
	sig := idx.signature(row)
	idx.sigs = append(idx.sigs, sig)
	idx.buckets[sig] = append(idx.buckets[sig], i)

	return i
}

func (idx *projectionIndex) update(i int, row []float32) {
	if idx == nil || i < 0 || i >= len(idx.sigs) {
		return
	}

	old := idx.sigs[i]
	removeBucketIndex(idx.buckets, old, i)

	sig := idx.signature(row)
	idx.sigs[i] = sig
	idx.buckets[sig] = append(idx.buckets[sig], i)
}

func (idx *projectionIndex) query(row []float32) []int {
	if idx == nil || len(idx.sigs) == 0 {
		return nil
	}

	sig := idx.signature(row)
	seen := make(map[int]struct{}, annBits+1)
	out := make([]int, 0, annBits*2)

	appendBucket := func(key uint32) {
		for _, i := range idx.buckets[key] {
			if _, ok := seen[i]; ok {
				continue
			}

			seen[i] = struct{}{}
			out = append(out, i)
		}
	}

	appendBucket(sig)

	for bit := range annBits {
		appendBucket(sig ^ (1 << uint(bit)))
	}

	return out
}

func removeBucketIndex(buckets map[uint32][]int, sig uint32, target int) {
	list := buckets[sig]
	for i, v := range list {
		if v != target {
			continue
		}

		list[i] = list[len(list)-1]
		buckets[sig] = list[:len(list)-1]

		if len(buckets[sig]) == 0 {
			delete(buckets, sig)
		}

		return
	}
}

func splitmixSeed(a, b uint64) uint64 {
	return splitmix64(a ^ (b + splitmixGamma))
}

func splitmix64(state uint64) uint64 {
	state += splitmixGamma
	z := state
	z = (z ^ (z >> splitmixMixShift1)) * splitmixMul1
	z = (z ^ (z >> splitmixMixShift2)) * splitmixMul2

	return z ^ (z >> splitmixFinalShift)
}

func collectANNRefs(idx *projectionIndex, row []float32, total int) []int {
	if total < annActivateAt {
		refs := make([]int, total)
		for i := range total {
			refs[i] = i
		}

		return refs
	}

	refs := idx.query(row)
	seen := make(map[int]struct{}, len(refs)+annRecentExactCap)
	out := make([]int, 0, len(refs)+annRecentExactCap)

	for _, i := range refs {
		if i < 0 || i >= total {
			continue
		}

		if _, ok := seen[i]; ok {
			continue
		}

		seen[i] = struct{}{}
		out = append(out, i)
	}

	start := max(0, total-annRecentExactCap)
	for i := start; i < total; i++ {
		if _, ok := seen[i]; ok {
			continue
		}

		seen[i] = struct{}{}
		out = append(out, i)
	}

	return out
}
