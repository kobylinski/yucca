package main

import (
	"context"
	"net/http"
	"testing"

	"github.com/kobylinski/yucca/internal/daemon"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
)

func TestDaemonStartsAndServesAPI(t *testing.T) {
	keyring.MockInit()
	// Isolate HOME so the daemon's lockfile (~/.yucca/daemon.json, derived
	// from os.UserHomeDir) lands in a temp dir — otherwise this test clobbers
	// and deletes the real running daemon's lockfile, knocking the tray app
	// offline whenever the suite runs.
	t.Setenv("HOME", t.TempDir())
	cfg := daemon.Config{
		StoreDir: t.TempDir(),
		Port:     0, // auto-assign
	}
	d, err := daemon.New(cfg)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = d.Start(ctx) }()
	<-d.Ready()

	resp, err := http.Get(d.Addr() + "/api/health")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	cancel()
}
