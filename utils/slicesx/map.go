// Copyright Sierra

package slicesx

func Map[I any, O any](ins []I, fn func(in I, idx int) O) []O {
	outs := make([]O, len(ins))
	for idx, in := range ins {
		outs[idx] = fn(in, idx)
	}
	return outs
}

// MapErr is similar to Map but allows for the mapping function to return an error
// If there is an error on map, MapErr will short circuit and return nil, error.
func MapErr[I any, O any](ins []I, fn func(in I, idx int) (O, error)) ([]O, error) {
	outs := make([]O, len(ins))
	for idx, in := range ins {
		outx, err := fn(in, idx)
		if err != nil {
			return nil, err
		}
		outs[idx] = outx
	}
	return outs, nil
}

func Every[I any](ins []I, fn func(in I, idx int) bool) bool {
	for idx, in := range ins {
		if !fn(in, idx) {
			return false
		}
	}
	return true
}

func ToSet[E comparable](in []E) map[E]struct{} {
	out := make(map[E]struct{})
	for _, e := range in {
		out[e] = struct{}{}
	}
	return out
}

func SetDifference[E comparable](first map[E]struct{}, second map[E]struct{}) map[E]struct{} {
	out := make(map[E]struct{})
	for k := range first {
		if _, ok := second[k]; !ok {
			out[k] = struct{}{}
		}
	}
	return out
}

func SetUpdate[E comparable](first map[E]struct{}, second map[E]struct{}) map[E]struct{} {
	for k := range second {
		first[k] = struct{}{}
	}
	return first
}
