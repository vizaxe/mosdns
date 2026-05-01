/*
 * Copyright (C) 2020-2022, IrineSistiana
 *
 * This file is part of mosdns.
 *
 * mosdns is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * mosdns is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

package mlog

import (
	"fmt"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"os"
	"strconv"
	"strings"
)

type LogConfig struct {
	// Level, See also zapcore.ParseLevel.
	Level string `yaml:"level"`

	// File that logger will be writen into.
	// Default is stderr.
	File string `yaml:"file"`

	// Production enables json output.
	Production bool `yaml:"production"`

	// Size limits log file size. Supports K, M, G suffix. Default unit is M.
	// When set, log file will be rotated automatically when exceeding this size.
	// Example: "100", "100M", "1G", "512K"
	Size string `yaml:"size"`

	// MaxBackups sets the maximum number of rotated log files to keep.
	// 0 means no backup files, the log file will be truncated and rewritten.
	// Default is 3.
	MaxBackups int `yaml:"max_backups"`
}

var (
	stderr = zapcore.Lock(os.Stderr)
	lvl    = zap.NewAtomicLevelAt(zap.InfoLevel)
	l      = zap.New(zapcore.NewCore(zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig()), stderr, lvl))
	s      = l.Sugar()

	nop = zap.NewNop()
)

// parseSize parses a size string like "100", "100M", "1G", "512K".
// If no unit suffix is provided, defaults to megabytes.
func parseSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return 0, fmt.Errorf("empty size")
	}

	s = strings.ToUpper(s)
	var multiplier int64 = 1024 * 1024 // default M

	last := s[len(s)-1]
	if last == 'K' {
		multiplier = 1024
		s = s[:len(s)-1]
	} else if last == 'M' {
		multiplier = 1024 * 1024
		s = s[:len(s)-1]
	} else if last == 'G' {
		multiplier = 1024 * 1024 * 1024
		s = s[:len(s)-1]
	}

	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size %q: %w", s, err)
	}
	if v <= 0 {
		return 0, fmt.Errorf("size must be positive, got %d", v)
	}

	return v * multiplier, nil
}

// rotateWriteSyncer implements zapcore.WriteSyncer with automatic log rotation.
type rotateWriteSyncer struct {
	path       string
	maxSize    int64
	maxBackups int
	file       *os.File
	size       int64
}

func newRotateWriteSyncer(path string, maxSize int64, maxBackups int) (*rotateWriteSyncer, error) {
	w := &rotateWriteSyncer{
		path:       path,
		maxSize:    maxSize,
		maxBackups: maxBackups,
	}
	if err := w.open(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *rotateWriteSyncer) open() error {
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("open log file %s: %w", w.path, err)
	}
	stat, err := f.Stat()
	if err != nil {
		f.Close()
		return fmt.Errorf("stat log file %s: %w", w.path, err)
	}
	w.file = f
	w.size = stat.Size()
	return nil
}

func (w *rotateWriteSyncer) Write(p []byte) (n int, err error) {
	if w.file == nil {
		if err := w.open(); err != nil {
			return 0, err
		}
	}

	n, err = w.file.Write(p)
	w.size += int64(n)
	if err != nil {
		return
	}

	if w.size >= w.maxSize {
		if err := w.rotate(); err != nil {
			return n, err
		}
	}
	return
}

func (w *rotateWriteSyncer) Sync() error {
	if w.file != nil {
		return w.file.Sync()
	}
	return nil
}

func (w *rotateWriteSyncer) Close() error {
	if w.file != nil {
		return w.file.Close()
	}
	return nil
}

func (w *rotateWriteSyncer) rotate() (err error) {
	if err = w.file.Close(); err != nil {
		return fmt.Errorf("close log file: %w", err)
	}

	if w.maxBackups > 0 {
		// Remove the oldest backup.
		os.Remove(w.path + fmt.Sprintf(".%d", w.maxBackups))

		// Shift backups: .N-1 -> .N, ..., .1 -> .2
		for i := w.maxBackups - 1; i >= 1; i-- {
			old := w.path + fmt.Sprintf(".%d", i)
			new := w.path + fmt.Sprintf(".%d", i+1)
			os.Rename(old, new)
		}

		// Rename current log to .1.
		if err = os.Rename(w.path, w.path+".1"); err != nil {
			return fmt.Errorf("rename log file: %w", err)
		}

		w.file = nil
		w.size = 0
		return w.open()
	}

	// No backups: truncate the file and reuse.
	f, err := os.Create(w.path)
	if err != nil {
		return fmt.Errorf("truncate log file: %w", err)
	}
	w.file = f
	w.size = 0
	return nil
}

func NewLogger(lc LogConfig) (*zap.Logger, error) {
	lvl, err := zapcore.ParseLevel(lc.Level)
	if err != nil {
		return nil, fmt.Errorf("invalid log level: %w", err)
	}

	var out zapcore.WriteSyncer
	if lf := lc.File; len(lf) > 0 {
		if sz := lc.Size; len(sz) > 0 {
			maxSize, err := parseSize(sz)
			if err != nil {
				return nil, fmt.Errorf("invalid log size: %w", err)
			}
			maxBackups := lc.MaxBackups
			if maxBackups < 0 {
				maxBackups = 0
			}
			out, err = newRotateWriteSyncer(lf, maxSize, maxBackups)
			if err != nil {
				return nil, fmt.Errorf("create rotate writer: %w", err)
			}
		} else {
			f, _, err := zap.Open(lf)
			if err != nil {
				return nil, fmt.Errorf("open log file: %w", err)
			}
			out = zapcore.Lock(f)
		}
	} else {
		out = stderr
	}

	if lc.Production {
		return zap.New(zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), out, lvl)), nil
	}
	return zap.New(zapcore.NewCore(zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig()), out, lvl)), nil
}

// L is a global logger.
func L() *zap.Logger {
	return l
}

// SetLevel sets the log level for the global logger.
func SetLevel(l zapcore.Level) {
	lvl.SetLevel(l)
}

// S is a global logger.
func S() *zap.SugaredLogger {
	return s
}

// Nop is a logger that never writes out logs.
func Nop() *zap.Logger {
	return nop
}
