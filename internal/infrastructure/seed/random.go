package seed

// random is a small deterministic PRNG (mulberry32) so the demo looks the
// same on every reset. Same algorithm as the web client's seed.
type random struct {
	t uint32
}

func newRandom(seed uint32) *random { return &random{t: seed} }

func (r *random) next() float64 {
	r.t += 0x6d2b79f5
	x := (r.t ^ (r.t >> 15)) * (1 | r.t)
	x ^= x + (x^(x>>7))*(61|x)
	return float64(x^(x>>14)) / 4294967296
}

func (r *random) chance(p float64) bool { return r.next() < p }

func (r *random) intn(lo, hi int) int {
	return int(r.next()*float64(hi-lo+1)) + lo
}

func pick[T any](r *random, items []T) T {
	return items[int(r.next()*float64(len(items)))]
}
