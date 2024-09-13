// Copyright Sierra

package slicesx

import (
	"cmp"
	"slices"

	"golang.org/x/exp/maps"
)

func RemoveElements[E comparable](in []E, rem []E) []E {
	if len(rem) == 0 {
		return in
	}
	out := make([]E, 0, len(in))
	toRem := ToSet(rem)

	for _, e := range in {
		if _, ok := toRem[e]; !ok {
			out = append(out, e)
		}
	}
	return out
}

func ToStrs[E ~string](in []E) []string {
	out := make([]string, len(in))
	for i, id := range in {
		out[i] = string(id)
	}
	return out
}

func ToAny[E any](in []E) []any {
	out := make([]any, len(in))
	for i, val := range in {
		out[i] = val
	}
	return out
}

// Dedupe returns a new slice with all duplicates removed while preserving order.
func Dedupe[E comparable](in ...[]E) []E {
	var size int
	for _, els := range in {
		size += len(els)
	}

	out := make([]E, 0, size)
	set := make(map[E]struct{}, size)

	for _, els := range in {
		for _, el := range els {
			if _, ok := set[el]; !ok {
				out = append(out, el)
				set[el] = struct{}{}
			}
		}
	}

	return out
}

// DedupeBy returns a new slice with all duplicates (two elements with the same
// key returned by the keyFn) removed while preserving order.
func DedupeBy[E any, K comparable](in []E, keyFn func(E) K) []E {
	out := make([]E, 0, len(in))
	set := make(map[K]struct{}, len(in))

	for _, el := range in {
		key := keyFn(el)
		if _, ok := set[key]; !ok {
			out = append(out, el)
			set[key] = struct{}{}
		}
	}

	return out
}

func Filter[E any](in []E, f func(E) bool) []E {
	out := make([]E, 0, len(in))
	for _, e := range in {
		if f(e) {
			out = append(out, e)
		}
	}
	return out
}

func FilterErr[E any](in []E, f func(E) (bool, error)) ([]E, error) {
	out := make([]E, 0, len(in))
	for _, e := range in {
		matched, err := f(e)
		if err != nil {
			return nil, err
		}
		if matched {
			out = append(out, e)
		}
	}
	return out, nil
}

// FilterOut is just like Filter except useful for cases where it reads better
// as a negative filter. e.g. FilterOut(listOfStrings, isEmpty) reads better
// than Filter(listOfStrings, isNonEmpty)
func FilterOut[E any](in []E, f func(E) bool) []E {
	return Filter(in, func(e E) bool {
		return !f(e)
	})
}

// Partition splits a slice and returns two slices.
// the first slice contains all elements where f is true
// the second slice contains the remainder
func Partition[E any](in []E, f func(E) bool) (lhs []E, rhs []E) {
	lhs = make([]E, 0, len(in))
	rhs = make([]E, 0, len(in))

	for _, e := range in {
		if f(e) {
			lhs = append(lhs, e)
		} else {
			rhs = append(rhs, e)
		}
	}

	return lhs, rhs
}

func Reverse[E any](in []E) []E {
	out := make([]E, len(in))
	for i := range in {
		out[len(in)-1-i] = in[i]
	}
	return out
}

func Pop[E any](in []E) ([]E, E, bool) {
	var e E
	if len(in) == 0 {
		return nil, e, false
	}

	e = in[len(in)-1]
	return in[:len(in)-1], e, true
}

func Includes[E comparable](els []E, s E) bool {
	return slices.Index(els, s) != -1
}

func Merge[S ~[]E, E any](a, b S) S {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}

	result := make(S, len(a)+len(b))
	n := copy(result, a)
	copy(result[n:], b)
	return result
}

func Chunk[O any](items []O, chunkSize int) [][]O {
	if len(items) <= chunkSize {
		return [][]O{items}
	}
	chunks := [][]O{}
	for start := 0; start < len(items); start += chunkSize {
		end := start + chunkSize
		if end > len(items) {
			end = len(items)
		}
		chunk := []O{}
		for i := start; i < end; i++ {
			chunk = append(chunk, items[i])
		}
		chunks = append(chunks, chunk)
	}
	return chunks
}

func Find[O any](els []O, fn func(o O) bool) (O, bool) {
	for _, el := range els {
		if fn(el) {
			return el, true
		}
	}
	var empty O
	return empty, false
}

func FindErr[O any](els []O, fn func(o O) (bool, error)) (O, bool, error) {
	var empty O
	for _, el := range els {
		ok, err := fn(el)
		if err != nil {
			return empty, false, err
		}
		if ok {
			return el, true, nil
		}
	}

	return empty, false, nil
}

func FindLast[O any](els []O, fn func(o O) bool) (O, bool) {
	for i := len(els) - 1; i >= 0; i-- {
		if fn(els[i]) {
			return els[i], true
		}
	}
	var empty O
	return empty, false
}

func SortByID[S ~[]E, E interface{ GetId() string }](x S) {
	slices.SortFunc(x, func(a, b E) int {
		return cmp.Compare(a.GetId(), b.GetId())
	})
}

func Intersection[E comparable](elss [][]E) []E {
	var set map[E]struct{}
	for i, els := range elss {
		iterSet := make(map[E]struct{})
		for _, id := range els {
			iterSet[id] = struct{}{}
		}
		if i == 0 {
			set = iterSet
		} else {
			for id := range set {
				if _, ok := iterSet[id]; !ok {
					delete(set, id)
				}
			}
		}
	}

	return maps.Keys(set)
}

func ClipElements[E any](els []E, size int) []E {
	if len(els) < size {
		return els
	}
	return els[:size]
}
