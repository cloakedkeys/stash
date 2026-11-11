package config

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestGenerationThreads(t *testing.T) {
	c := InitializeEmpty()
	c.setDefaultValues()
	require.Equal(t, 4, c.GetGenerationThreads())
	c.SetInterface(GenerationThreads, 7)
	require.Equal(t, 7, c.GetGenerationThreads())
	c.SetInterface(GenerationThreads, 0)
	require.Equal(t, 0, c.GetGenerationThreads())
}
