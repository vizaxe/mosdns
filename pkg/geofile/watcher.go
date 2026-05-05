package geofile

import (
	"context"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"
)

const watchDelay = 200 * time.Millisecond

// WatchFile monitors filePath for write/create events.
// When a change is detected, it waits briefly for the write to finish,
// then calls onReload. If onReload returns an error, the old state is kept.
// The watcher runs until ctx is cancelled.
func WatchFile(ctx context.Context, logger *zap.Logger, filePath string, onReload func() error) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return err
	}

	dir := filepath.Dir(absPath)
	if err := watcher.Add(dir); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			if event.Name != absPath {
				continue
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				time.Sleep(watchDelay)
				logger.Info("file changed, reloading", zap.String("file", absPath))
				if err := onReload(); err != nil {
					logger.Error("failed to reload file, keeping old data", zap.String("file", absPath), zap.Error(err))
				} else {
					logger.Info("file reloaded successfully", zap.String("file", absPath))
				}
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			logger.Error("file watcher error", zap.Error(err))
		}
	}
}
