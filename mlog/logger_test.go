package mlog

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
)

func TestParseSize(t *testing.T) {
	tests := []struct {
		input string
		want  int64
		err   bool
	}{
		{"100", 100 * 1024 * 1024, false},
		{"100M", 100 * 1024 * 1024, false},
		{"1G", 1 * 1024 * 1024 * 1024, false},
		{"512K", 512 * 1024, false},
		{" 200 ", 200 * 1024 * 1024, false},
		{"0", 0, true},
		{"-1", 0, true},
		{"abc", 0, true},
		{"", 0, true},
	}
	for _, tt := range tests {
		got, err := parseSize(tt.input)
		if tt.err {
			assert.Error(t, err, "input: %s", tt.input)
		} else {
			assert.NoError(t, err, "input: %s", tt.input)
			assert.Equal(t, tt.want, got, "input: %s", tt.input)
		}
	}
}

func TestRotateWriteSyncer(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")

	// Create syncer with 100 byte max size, 3 backups
	w, err := newRotateWriteSyncer(logPath, 100, 3)
	require.NoError(t, err)
	defer w.Close()

	var ws zapcore.WriteSyncer = w

	n, err := ws.Write([]byte("hello\n"))
	require.NoError(t, err)
	assert.Equal(t, 6, n)

	big := make([]byte, 200)
	for i := range big {
		big[i] = 'a'
	}
	n, err = ws.Write(big)
	require.NoError(t, err)
	assert.Equal(t, 200, n)
	ws.Sync()

	_, err = os.Stat(logPath + ".1")
	assert.NoError(t, err, "backup file .1 should exist")

	n, err = ws.Write(big)
	require.NoError(t, err)
	assert.Equal(t, 200, n)
	ws.Sync()

	_, err = os.Stat(logPath + ".2")
	assert.NoError(t, err, "backup file .2 should exist")
}

func TestRotateWriteSyncerNoBackup(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "nobackup.log")

	w, err := newRotateWriteSyncer(logPath, 100, 0)
	require.NoError(t, err)
	defer w.Close()

	big := make([]byte, 200)
	for i := range big {
		big[i] = 'a'
	}

	_, err = w.Write(big)
	require.NoError(t, err)
	w.Sync()

	// No .1 backup should exist
	_, err = os.Stat(logPath + ".1")
	assert.True(t, os.IsNotExist(err), "backup file .1 should NOT exist")

	// File should still exist and be writable
	_, err = os.Stat(logPath)
	assert.NoError(t, err)

	// Trigger second rotation
	_, err = w.Write(big)
	require.NoError(t, err)
	w.Sync()

	// Still no backup files
	_, err = os.Stat(logPath + ".1")
	assert.True(t, os.IsNotExist(err))
}

func TestNewLoggerWithSize(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "size-test.log")

	lc := LogConfig{
		Level: "debug",
		File:  logPath,
		Size:  "1K",
	}

	lg, err := NewLogger(lc)
	require.NoError(t, err)
	require.NotNil(t, lg)

	lg.Info("test log message")

	// Verify file was created
	stat, err := os.Stat(logPath)
	require.NoError(t, err)
	assert.Greater(t, stat.Size(), int64(0))
}
