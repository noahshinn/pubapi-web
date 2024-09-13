// Copyright Sierra

package slicesx

import (
	"errors"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRemoveElements(t *testing.T) {
	els := []int{1, 2, 3, 4, 5}

	assert.ElementsMatch(t, RemoveElements(els, []int{1, 3, 5}), []int{2, 4})
}

func TestFilter(t *testing.T) {
	els := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}

	assert.ElementsMatch(t, Filter(els, func(n int) bool {
		return n%2 == 0
	}), []int{2, 4, 6, 8, 10})
}

func TestFilterOut(t *testing.T) {
	els := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}

	assert.ElementsMatch(t, FilterOut(els, func(n int) bool {
		return n%2 == 0
	}), []int{1, 3, 5, 7, 9})
}

func TestFilterErr(t *testing.T) {
	els := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	filtered, err := FilterErr(els, func(n int) (bool, error) {
		return n%2 == 1, nil
	})
	assert.NoError(t, err)
	assert.ElementsMatch(t, filtered, []int{1, 3, 5, 7, 9})

	filtered, err = FilterErr(els, func(n int) (bool, error) {
		return false, errors.New("error")
	})
	assert.Error(t, err)
	assert.Nil(t, filtered)
}

func TestPartition(t *testing.T) {
	els := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	lhs, rhs := Partition(els, func(n int) bool {
		return n%2 == 0
	})

	assert.ElementsMatch(t, lhs, []int{2, 4, 6, 8, 10})
	assert.ElementsMatch(t, rhs, []int{1, 3, 5, 7, 9})
}

func TestRemoveElementsDupes(t *testing.T) {
	els := []int{1, 2, 3, 4, 5, 5, 5, 5, 5, 3, 1}

	assert.ElementsMatch(t, RemoveElements(els, []int{1, 3, 5}), []int{2, 4})
}

func TestToStrs(t *testing.T) {
	type s string
	els := []s{"1", "2", "3", "4", "5"}
	assert.ElementsMatch(t, ToStrs(els), []string{"1", "2", "3", "4", "5"})
}

func TestDedupe(t *testing.T) {
	els := []int{1, 3, 2, 3, 4, 5, 5, 5, 5, 5, 3, 1}

	assert.ElementsMatch(t, Dedupe(els), []int{1, 3, 2, 4, 5})
}

func TestDedupeBy(t *testing.T) {
	els := []int{10, 1, 2, 20, 24, 3, 4}

	assert.ElementsMatch(t, DedupeBy(els, func(n int) string {
		return strconv.Itoa(n)[:1]
	}), []int{10, 2, 3, 4})
}

func TestDedupeManySlices(t *testing.T) {
	els := []int{1, 3, 2, 3, 4, 5, 5, 5, 5, 5, 3, 1}
	els2 := []int{3, 5, 1, 5, 7, 2}

	assert.ElementsMatch(t, Dedupe(els, els2), []int{1, 3, 2, 4, 5, 7})
}

func TestReverse(t *testing.T) {
	els := []int{1, 2, 3}

	assert.ElementsMatch(t, Reverse(els), []int{3, 2, 1})
}

func TestPop(t *testing.T) {
	els := []int{1, 2, 3}

	var el int
	var ok bool
	for _, expected := range []int{3, 2, 1} {
		els, el, ok = Pop(els)
		assert.Equal(t, expected, el)
		assert.True(t, ok)
	}
	_, el, ok = Pop(els)
	assert.Equal(t, 0, el)
	assert.False(t, ok)
}

type User struct {
	Id string
}

func (x *User) GetId() string {
	if x != nil {
		return x.Id
	}
	return ""
}

func TestSortByIDs(t *testing.T) {
	assert := assert.New(t)

	s := []*User{
		{Id: "c"},
		{Id: "b"},
		{Id: "a"},
	}
	SortByID(s)
	assert.Equal("a", s[0].Id)
	assert.Equal("b", s[1].Id)
	assert.Equal("c", s[2].Id)
}

func TestFind(t *testing.T) {
	els := []int{1, 2, 3, 4, 5}

	el, ok := Find(els, func(el int) bool {
		return el == 3
	})
	assert.True(t, ok)
	assert.Equal(t, 3, el)

	el, ok = Find(els, func(el int) bool {
		return el == 6
	})
	assert.False(t, ok)
	assert.Equal(t, 0, el)
}

func TestFindErr(t *testing.T) {
	els := []int{1, 2, 3, 4, 5}

	el, ok, err := FindErr(els, func(el int) (bool, error) {
		return el == 3, nil
	})
	assert.True(t, ok)
	assert.Equal(t, 3, el)
	assert.NoError(t, err)

	el, ok, err = FindErr(els, func(el int) (bool, error) {
		return false, errors.New("err")
	})

	assert.Error(t, err)
	assert.False(t, ok)
	assert.Equal(t, 0, el)
}

func TestFindLast(t *testing.T) {
	els := []int{1, 2, 3, 4, 5}

	el, ok := FindLast(els, func(el int) bool {
		return el == 5
	})
	assert.True(t, ok)
	assert.Equal(t, 5, el)

	el, ok = Find(els, func(el int) bool {
		return el == 6
	})
	assert.False(t, ok)
	assert.Equal(t, 0, el)
}

func TestIntersection(t *testing.T) {
	elss := [][]int{
		{1, 2, 3, 4},
		{1, 2, 4},
		{1, 2, 3, 5},
	}

	assert.ElementsMatch(t, Intersection(elss), []int{1, 2})
}
