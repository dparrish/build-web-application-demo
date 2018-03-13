// Package autoconfig wraps a JSON configuration stored on disk that is queryable using the Get* functions.
//
// The configuration file will be watched for changes after the initial load. Whenever the file has changed, each
// validation function will be called in the order they were added.
package autoconfig

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/clbanning/mxj"
	"github.com/go-fsnotify/fsnotify"
	"github.com/spf13/afero"
)

var Fs = afero.NewOsFs()

// Config wraps a JSON configuration stored on disk and provides functions to query it.
type Config struct {
	sync.RWMutex
	filename   string
	mv         mxj.Map
	validators []func(old *Config, new *Config) error
}

func Load(ctx context.Context, filename string) (*Config, error) {
	c := &Config{filename: filename}
	if err := c.read(); err != nil {
		return nil, fmt.Errorf("unable to read initial config: %v", err)
	}
	return c, nil
}

func (c *Config) Watch(ctx context.Context) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("couldn't create config watcher: %v", err)
	}
	if err := watcher.Add(c.filename); err != nil {
		return fmt.Errorf("couldn't create config watcher: %v", err)
	}
	go c.background(ctx, watcher)
	return nil
}

// AddValidator adds a function that will be called whenever the config file changes.
// The function will be passed both the old and new configurations. If the function returns an error, the new
// configuration will not be applied.
// The validation function *may* modify the new config but *must not* modify the old config.
func (c *Config) AddValidator(f func(old *Config, new *Config) error) {
	c.Lock()
	c.validators = append(c.validators, f)
	c.Unlock()
}

// Get looks up a configuration item in dotted path notation and returns the first (or only) value.
// Example: c.Get("spanner.database.path")
func (c *Config) Get(path string) string {
	c.RLock()
	defer c.RUnlock()
	values, err := c.mv.ValuesForPath(path)
	if err != nil {
		log.Printf("Error in ValuesForPath(%q): %v", path, err)
	}
	if len(values) == 0 {
		return ""
	}
	return values[0].(string)
}

// Get looks up a configuration item in dotted path notation and returns a list of values.
func (c *Config) GetAll(path string) []string {
	c.RLock()
	defer c.RUnlock()
	values, err := c.mv.ValuesForPath(path)
	if err != nil {
		log.Printf("Error in ValuesForPath(%q): %v", path, err)
	}
	r := make([]string, 0, len(values))
	for _, v := range values {
		r = append(r, v.(string))
	}
	return r
}

func (c *Config) read() error {
	body, err := afero.ReadFile(Fs, c.filename)
	if err != nil {
		return fmt.Errorf("couldn't read config file %q: %v", c.filename, err)
	}
	mv, err := mxj.NewMapJson(body)
	if err != nil {
		return fmt.Errorf("couldn't parse config: %v", err)
	}

	newConfig := &Config{mv: mv}
	for _, f := range c.validators {
		if err := f(c, newConfig); err != nil {
			log.Printf("Config validation failed: %v", err)
			return err
		}
	}

	c.Lock()
	c.mv = mv
	c.Unlock()
	return nil
}

func (c *Config) background(ctx context.Context, watcher *fsnotify.Watcher) {
	defer watcher.Close()
	t := make(<-chan time.Time)
	for {
		select {
		case <-ctx.Done():
			// Stop watching when the context is cancelled.
			return
		case _, ok := <-watcher.Events:
			if !ok {
				log.Printf("Watcher ended for %q", c.filename)
				return
			}
			// Create a timer to re-read the config file one second after noticing an event. This prevents the config file
			// being read multiple times for a single file change.
			t = time.After(1 * time.Second)
			// Re-watch the file for further changes.
			watcher.Add(c.filename)
		case <-t:
			if err := c.read(); err != nil {
				log.Printf("Error re-reading config file, keeping existing config: %v", err)
			} else {
				log.Printf("Read changed config file %q", c.filename)
			}
		}
	}
}
