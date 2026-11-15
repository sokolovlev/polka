package kv_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sokolovlev/polka/libs/go/kv"
	"github.com/sokolovlev/polka/libs/go/kvtest"
)

func TestClientAndCheck(t *testing.T) {
	ctx := context.Background()
	c := kvtest.New(t)

	require.NoError(t, kv.Check(c).Check(ctx))
	require.NoError(t, c.Set(ctx, "k", "v", time.Minute).Err())
	got, err := c.Get(ctx, "k").Result()
	require.NoError(t, err)
	assert.Equal(t, "v", got)

	other := kvtest.New(t)
	n, err := other.Exists(ctx, "k").Result()
	require.NoError(t, err)
	assert.Zero(t, n, "each test gets its own logical database")

	require.NoError(t, c.Close())
	require.Error(t, kv.Check(c).Check(ctx))
}

func TestConnectFailsFast(t *testing.T) {
	_, err := kv.Connect(context.Background(), kv.Config{ValkeyAddr: "127.0.0.1:1"})
	require.Error(t, err)
}
