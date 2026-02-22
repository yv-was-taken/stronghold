package db

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewFromPool_PanicsOnNil(t *testing.T) {
	assert.Panics(t, func() {
		NewFromPool(nil)
	})
}
