package config

import (
	"os"
	"time"
)

// writeFileAtomic replaces path in one step, so a crash or a concurrent reader
// sees either the old content or the new one, never a truncated file.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = replaceFile(tmp, path)
	}
	if err != nil {
		_ = os.Remove(tmp)
	}
	return err
}

// On Windows a scanner or indexer holding the target open blocks the rename
// for a moment.
func replaceFile(from, to string) error {
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		if attempt > 0 {
			time.Sleep(20 * time.Millisecond)
		}
		if err = os.Rename(from, to); err == nil {
			return nil
		}
	}
	return err
}
