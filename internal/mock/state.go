package mock

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// saveState writes fixture to path as indented JSON, replacing the file
// atomically so a crash cannot leave a truncated state file behind.
func saveState(path string, fixture *Fixture) error {
	data, err := json.MarshalIndent(fixture, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling dev state: %w", err)
	}
	data = append(data, '\n')

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("creating dev state directory: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".tira-dev-state-*")
	if err != nil {
		return fmt.Errorf("creating dev state temp file: %w", err)
	}
	tmpName := tmp.Name()

	discard := func() { _ = os.Remove(tmpName) }

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		discard()
		return fmt.Errorf("writing dev state: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		discard()
		return fmt.Errorf("syncing dev state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		discard()
		return fmt.Errorf("closing dev state: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		discard()
		return fmt.Errorf("replacing dev state: %w", err)
	}
	return nil
}

// loadState reads a state file. ok is false when the file does not exist or is
// empty, so the caller falls back to the fixture and leaves the state file
// untouched until the first mutation.
func loadState(path string) (fixture *Fixture, ok bool, err error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("reading dev state %q: %w", path, err)
	}
	if info.IsDir() {
		return nil, false, fmt.Errorf("dev state path %q is a directory; pass a single JSON file", path)
	}
	if info.Size() == 0 {
		return nil, false, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false, fmt.Errorf("reading dev state %q: %w", path, err)
	}

	fixture, err = decode(data)
	if err != nil {
		return nil, false, fmt.Errorf("dev state %q: %w", path, err)
	}
	if err := fixture.validate(); err != nil {
		return nil, false, fmt.Errorf("invalid dev state %q: %w", path, err)
	}
	return fixture, true, nil
}
