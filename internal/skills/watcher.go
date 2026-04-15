package skills

import (
	"fmt"
	"os"
	"sync"
	"time"
)

// Watcher monitors skill directories for changes and triggers a reload of
// the registry. It uses a polling approach (no external dependencies) with
// debounce to avoid reloading multiple times for a burst of file changes.
type Watcher struct {
	registry *Registry
	dirs     map[Source]string
	interval time.Duration
	debounce time.Duration

	stopCh chan struct{}
	wg     sync.WaitGroup

	// OnReload is an optional callback invoked after each successful reload.
	OnReload func(count int)
}

// WatcherOptions configures the file watcher.
type WatcherOptions struct {
	// PollInterval is how often to check for changes. Default: 2s.
	PollInterval time.Duration
	// DebounceWindow is how long to wait after the last change before
	// triggering a reload. Default: 500ms.
	DebounceWindow time.Duration
}

// NewWatcher creates a watcher that monitors the given directories for
// changes and reloads the registry when they occur.
func NewWatcher(registry *Registry, dirs map[Source]string, opts WatcherOptions) *Watcher {
	if opts.PollInterval <= 0 {
		opts.PollInterval = 2 * time.Second
	}
	if opts.DebounceWindow <= 0 {
		opts.DebounceWindow = 500 * time.Millisecond
	}

	return &Watcher{
		registry: registry,
		dirs:     dirs,
		interval: opts.PollInterval,
		debounce: opts.DebounceWindow,
		stopCh:   make(chan struct{}),
	}
}

// Start begins watching directories in a background goroutine.
func (w *Watcher) Start() {
	w.wg.Add(1)
	go w.loop()
}

// Stop terminates the watcher and waits for it to finish.
func (w *Watcher) Stop() {
	close(w.stopCh)
	w.wg.Wait()
}

func (w *Watcher) loop() {
	defer w.wg.Done()

	snapshot := w.takeSnapshot()
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-w.stopCh:
			return
		case <-ticker.C:
			current := w.takeSnapshot()
			if !snapshotsEqual(snapshot, current) {
				// Debounce: wait a bit, then take another snapshot to make
				// sure the burst of writes is done.
				select {
				case <-w.stopCh:
					return
				case <-time.After(w.debounce):
				}

				current = w.takeSnapshot()
				n, err := w.registry.Reload(w.dirs)
				if err != nil {
					fmt.Fprintf(os.Stderr, "skills watcher reload error: %v\n", err)
				} else if w.OnReload != nil {
					w.OnReload(n)
				}
				snapshot = current
			}
		}
	}
}

// fileSnapshot records a file's modification time and size.
type fileSnapshot struct {
	modTime time.Time
	size    int64
}

// takeSnapshot collects mod times for all SKILL.md files across watched dirs.
func (w *Watcher) takeSnapshot() map[string]fileSnapshot {
	snap := make(map[string]fileSnapshot)
	for _, dir := range w.dirs {
		if dir == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			skillFile := dir + "/" + e.Name() + "/SKILL.md"
			info, err := os.Stat(skillFile)
			if err != nil {
				continue
			}
			snap[skillFile] = fileSnapshot{
				modTime: info.ModTime(),
				size:    info.Size(),
			}
		}
	}
	return snap
}

func snapshotsEqual(a, b map[string]fileSnapshot) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		bv, ok := b[k]
		if !ok || v.modTime != bv.modTime || v.size != bv.size {
			return false
		}
	}
	return true
}
