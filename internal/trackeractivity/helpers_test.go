package trackeractivity

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAddressOf(t *testing.T) {
	require.Equal(t, "foo", *addressOf("foo"))
	require.Equal(t, "bar", *addressOf("bar"))
}
