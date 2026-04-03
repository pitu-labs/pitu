package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsAllowed_EmptyList_AllowsAll(t *testing.T) {
	assert.True(t, isAllowed(12345, nil))
	assert.True(t, isAllowed(12345, []int64{}))
}

func TestIsAllowed_AllowedID_ReturnsTrue(t *testing.T) {
	list := []int64{111, 222, 333}
	assert.True(t, isAllowed(111, list)) // first element
	assert.True(t, isAllowed(222, list)) // middle element
	assert.True(t, isAllowed(333, list)) // last element
}

func TestIsAllowed_UnknownID_ReturnsFalse(t *testing.T) {
	list := []int64{111, 222, 333}
	assert.False(t, isAllowed(999, list))
}
