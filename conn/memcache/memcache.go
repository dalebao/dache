// Package memcacheconn provides a Memcached-based Connector.
package memcacheconn

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/bradfitz/gomemcache/memcache"
)

// Connector implements localcache.Connector using Memcached.
// "table" in ScanAll is treated as a key name.
type Connector struct {
	name   string
	client *memcache.Client
	opts   Options
}

// Options configures the Memcached Connector.
type Options struct {
	// KeyPrefix is prepended to table names. Default: "dache:".
	KeyPrefix string
	// KeySuffix is appended to table names.
	KeySuffix string
}

// New creates a Memcached Connector from an existing *memcache.Client.
func New(name string, client *memcache.Client, opts Options) *Connector {
	if opts.KeyPrefix == "" {
		opts.KeyPrefix = "dache:"
	}
	return &Connector{name: name, client: client, opts: opts}
}

func (c *Connector) Name() string { return c.name }

// ScanAll reads a Memcached key and unmarshals the JSON value into dest.
func (c *Connector) ScanAll(ctx context.Context, table string, dest any) error {
	key := c.opts.KeyPrefix + table + c.opts.KeySuffix

	item, err := c.client.Get(key)
	if err != nil {
		return fmt.Errorf("memcacheconn %s: get key %q: %w", c.name, key, err)
	}

	if err := json.Unmarshal(item.Value, dest); err != nil {
		return fmt.Errorf("memcacheconn %s: unmarshal key %q: %w", c.name, key, err)
	}

	return nil
}

// ScanAllMulti reads multiple keys and aggregates into a slice.
// Keys are read in parallel using GetMulti.
func (c *Connector) ScanAllMulti(ctx context.Context, keys []string, dest any) error {
	prefixed := make([]string, len(keys))
	for i, k := range keys {
		prefixed[i] = c.opts.KeyPrefix + k + c.opts.KeySuffix
	}

	result, err := c.client.GetMulti(prefixed)
	if err != nil {
		return fmt.Errorf("memcacheconn %s: get multi: %w", c.name, err)
	}

	destVal := reflect.ValueOf(dest)
	if destVal.Kind() != reflect.Ptr || destVal.Elem().Kind() != reflect.Slice {
		return fmt.Errorf("memcacheconn: dest must be pointer to slice")
	}
	sliceVal := destVal.Elem()
	elemType := sliceVal.Type().Elem()
	isPtr := elemType.Kind() == reflect.Ptr
	if isPtr {
		elemType = elemType.Elem()
	}

	for _, item := range result {
		elem := reflect.New(elemType)
		if err := json.Unmarshal(item.Value, elem.Interface()); err != nil {
			continue
		}
		if isPtr {
			sliceVal = reflect.Append(sliceVal, elem.Elem())
		} else {
			sliceVal = reflect.Append(sliceVal, elem.Elem())
		}
	}

	destVal.Elem().Set(sliceVal)
	return nil
}

func (c *Connector) Ping(ctx context.Context) error {
	return c.client.Ping()
}

func (c *Connector) Close() error {
	// memcache.Client has no Close method, but we provide a no-op
	return nil
}
