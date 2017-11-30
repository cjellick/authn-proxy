package config

import (
	"bufio"
	"context"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type ParseType int

const (
	SingleValueFile ParseType = iota
	PropertiesFile
)

// Manages watching property files and providing access to the the key/values
// Two file formats are supported:
// - simple properties files with newline terminated k=v
// - a Single value files where the entire value of the file is value and the base name of the file is the key
// Since files are watched for changes, no garauntees are made about precedence with regard to duplicate keys.
// The last key loaded wins.

var m *Manager
var mutex sync.Mutex

func GetManager(ctx context.Context) *Manager {
	mutex.Lock()
	defer mutex.Unlock()

	if m == nil {
		m = &Manager{
			ctx:          ctx,
			m:            &sync.RWMutex{},
			config:       map[string]string{},
			watchedFiles: map[string]bool{},
		}
	}

	return m
}

type Manager struct {
	ctx          context.Context
	m            *sync.RWMutex
	config       map[string]string
	watchedFiles map[string]bool
}

type watcher struct {
	path      string
	fsNotify  *fsnotify.Watcher
	parseType ParseType
	mgr       *Manager
}

func (m *Manager) AddConfigFile(path string, parseType ParseType) error {
	if parseType != SingleValueFile && parseType != PropertiesFile {
		return errors.Errorf("Bad parseType %v", parseType)
	}

	m.m.Lock()
	if m.watchedFiles[path] {
		m.m.Unlock()
		return nil
	}
	m.watchedFiles[path] = true
	m.m.Unlock()

	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return errors.Wrapf(err, "couldn't create watcher for file %v", path)
	}

	watcher := &watcher{
		path:      path,
		parseType: parseType,
		fsNotify:  fsWatcher,
		mgr:       m,
	}

	if err := watcher.parse(); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return errors.Wrap(err, "error parsing file")
	}

	// Add the file to be watched
	if err := fsWatcher.Add(path); err != nil {
		return errors.Wrap(err, "error watching file")
	}

	// Launch a go thread to watch the file
	go watcher.run()
	return nil
}

func (m *Manager) Get(key string) string {
	m.m.RLock()
	defer m.m.RUnlock()
	v := m.config[key]
	return v
}

func (w *watcher) run() {
	for {
		select {
		case event := <-w.fsNotify.Events:
			if event.Op == fsnotify.Remove {
				w.fsNotify.Remove(event.Name)
				w.fsNotify.Add(event.Name)
			} else if event.Op == fsnotify.Write {
				if err := w.parse(); err != nil {
					logrus.Errorf("Error parsing config file %v: %v", w.path, err)
				}
			}
		case <-w.mgr.ctx.Done():
			w.fsNotify.Close()
			return
		}
	}
}

func (w *watcher) parse() error {
	if w.parseType == SingleValueFile {
		bytes, err := ioutil.ReadFile(w.path)
		if err != nil {
			return errors.Wrap(err, "couldn't read single value file")
		}

		w.mgr.m.Lock()
		defer w.mgr.m.Unlock()
		w.mgr.config[path.Base(w.path)] = string(bytes)
		return nil
	}

	f, err := os.Open(w.path)
	if err != nil {
		return err
	}
	defer f.Close()

	tmp := map[string]string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 && !strings.HasPrefix(parts[0], "#") {
			tmp[parts[0]] = parts[1]
		}
	}

	w.mgr.m.Lock()
	defer w.mgr.m.Unlock()
	for k, v := range tmp {
		w.mgr.config[k] = v
	}

	return nil
}
