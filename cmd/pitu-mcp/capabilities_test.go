package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseCapabilities(t *testing.T) {
	assert.Equal(t, []string{"gmail", "gcalendar"}, parseCapabilities("gmail,gcalendar"))
	assert.Equal(t, []string{"gmail"}, parseCapabilities(" gmail "))
	assert.Empty(t, parseCapabilities(""))
}

func TestIsCapabilityEnabled(t *testing.T) {
	caps := []string{"gmail", "noop"}
	assert.True(t, isCapabilityEnabled(caps, "noop"))
	assert.False(t, isCapabilityEnabled(caps, "gcalendar"))
}
