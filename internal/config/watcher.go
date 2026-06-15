package config

import (
	"log"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// ChangeHandler is called when the config file changes.
type ChangeHandler func(oldCfg, newCfg Config)

// Watcher watches the config file and fires ChangeHandler callbacks on changes.
type Watcher struct {
	cfg      *Config
	handlers []ChangeHandler
	watcher  *fsnotify.Watcher
	stopCh   chan struct{}
	mu       sync.Mutex
	lastHash string
	running  bool
}

// NewWatcher creates a Watcher for the given config.
func NewWatcher(cfg *Config) *Watcher {
	return &Watcher{cfg: cfg, stopCh: make(chan struct{})}
}

// OnChange registers a callback invoked after a config reload.
func (w *Watcher) OnChange(h ChangeHandler) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.handlers = append(w.handlers, h)
}

// Start begins watching. Returns an error if the file path is unknown.
func (w *Watcher) Start() error {
	path := w.cfg.FilePath()
	if path == "" {
		return nil // no file to watch
	}

	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	if err := fw.Add(path); err != nil {
		fw.Close()
		return err
	}

	w.mu.Lock()
	w.watcher = fw
	w.lastHash = w.cfg.Hash()
	w.running = true
	w.mu.Unlock()

	go w.loop(fw, path)
	log.Printf("[config-watcher] watching %s", path)
	return nil
}

// Stop halts the watcher.
func (w *Watcher) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.running {
		return
	}
	close(w.stopCh)
	if w.watcher != nil {
		w.watcher.Close()
	}
	w.running = false
}

func (w *Watcher) loop(fw *fsnotify.Watcher, path string) {
	debounce := time.NewTimer(0)
	<-debounce.C // drain initial tick

	for {
		select {
		case <-w.stopCh:
			return

		case event, ok := <-fw.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
				continue
			}
			// Debounce: wait 2 seconds of quiet before reloading
			debounce.Reset(2 * time.Second)

		case err, ok := <-fw.Errors:
			if !ok {
				return
			}
			log.Printf("[config-watcher] error: %v", err)

		case <-debounce.C:
			w.reload()
		}
	}
}

func (w *Watcher) reload() {
	newHash := w.cfg.Hash()
	w.mu.Lock()
	if newHash == w.lastHash {
		w.mu.Unlock()
		return
	}
	oldCfg := *w.cfg
	w.mu.Unlock()

	if err := w.cfg.Reload(); err != nil {
		log.Printf("[config-watcher] reload error: %v", err)
		return
	}

	w.mu.Lock()
	w.lastHash = newHash
	handlers := make([]ChangeHandler, len(w.handlers))
	copy(handlers, w.handlers)
	w.mu.Unlock()

	newCfg := *w.cfg
	for _, h := range handlers {
		h(oldCfg, newCfg)
	}
	log.Printf("[config-watcher] config reloaded")
}
