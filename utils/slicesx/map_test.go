// Copyright Sierra

package slicesx

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMap(t *testing.T) {
	addOne := func(in int, idx int) int {
		return in + 1
	}

	els := []int{1, 2, 3, 4, 5}

	assert.ElementsMatch(t, Map(els, addOne), []int{2, 3, 4, 5, 6})
}

func TestMapErr(t *testing.T) {
	willError := func(in int, idx int) (int, error) {
		return 0, errors.New("new error")
	}
	els := []int{1, 2, 3, 4, 5}

	values, err := MapErr(els, willError)
	assert.Error(t, err)
	assert.Nil(t, values)

	addOne := func(in int, idx int) (int, error) {
		return in + 1, nil
	}

	values, err = MapErr(els, addOne)
	assert.NoError(t, err)
	assert.ElementsMatch(t, values, []int{2, 3, 4, 5, 6})
}

func TestEvery(t *testing.T) {
	isEven := func(in int, idx int) bool {
		return in%2 == 0
	}

	els := []int{1, 2, 3, 4, 5}
	assert.False(t, Every(els, isEven))

	els = []int{2, 4, 6, 8, 10}
	assert.True(t, Every(els, isEven))

	els = []int{}
	assert.True(t, Every(els, isEven))
}

func TestToSet(t *testing.T) {
	els := []int{1, 2, 3, 4, 5}

	set := ToSet(els)
	assert.Len(t, set, len(els))
	for _, el := range els {
		_, ok := set[el]
		assert.True(t, ok)
	}
}
