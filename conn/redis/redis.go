// Package redisconn provides a Redis-based Connector.
package redisconn

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/redis/go-redis/v9"
)

// Connector implements localcache.Connector using Redis.
// "table" in ScanAll is treated as a key pattern. If the value is a JSON string,
// it is unmarshalled into the destination slice.
type Connector struct {
	name   string
	client *redis.Client
	opts   Options
}

// Options configures the Redis Connector.
type Options struct {
	// KeyPrefix is prepended to table names when constructing Redis keys. Default: "".
	KeyPrefix string
	// KeySuffix is appended to table names when constructing Redis keys. Default: "".
	KeySuffix string
}

// New creates a Redis Connector from an existing *redis.Client.
func New(name string, client *redis.Client, opts Options) *Connector {
	if opts.KeyPrefix == "" {
		opts.KeyPrefix = "dache:"
	}
	return &Connector{name: name, client: client, opts: opts}
}

func (c *Connector) Name() string { return c.name }

// ScanAll reads a Redis key and unmarshals the JSON value into dest.
// table is used as the key name (with optional prefix/suffix).
func (c *Connector) ScanAll(ctx context.Context, table string, dest any) error {
	key := c.opts.KeyPrefix + table + c.opts.KeySuffix

	data, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		return fmt.Errorf("redisconn %s: get key %q: %w", c.name, key, err)
	}

	if err := json.Unmarshal(data, dest); err != nil {
		return fmt.Errorf("redisconn %s: unmarshal key %q: %w", c.name, key, err)
	}

	return nil
}

// ScanAllPattern iterates over keys matching a pattern and aggregates into dest.
func (c *Connector) ScanAllPattern(ctx context.Context, pattern string, dest any) error {
	iter := c.client.Scan(ctx, 0, c.opts.KeyPrefix+pattern+c.opts.KeySuffix, 0).Iterator()

	destVal := reflect.ValueOf(dest)
	if destVal.Kind() != reflect.Ptr || destVal.Elem().Kind() != reflect.Slice {
		return fmt.Errorf("redisconn %s: dest must be pointer to slice", c.name)
	}
	sliceVal := destVal.Elem()
	elemType := sliceVal.Type().Elem()
	isPtr := elemType.Kind() == reflect.Ptr
	if isPtr {
		elemType = elemType.Elem()
	}

	for iter.Next(ctx) {
		data, err := c.client.Get(ctx, iter.Val()).Bytes()
		if err != nil {
			continue
		}

		if isPtr {
			elem := reflect.New(elemType)
			if err := json.Unmarshal(data, elem.Interface()); err != nil {
				continue
			}
			sliceVal = reflect.Append(sliceVal, elem.Elem())
		} else {
			elem := reflect.New(elemType)
			if err := json.Unmarshal(data, elem.Interface()); err != nil {
				continue
			}
			sliceVal = reflect.Append(sliceVal, elem.Elem())
		}
	}

	destVal.Elem().Set(sliceVal)
	return iter.Err()
}

func (c *Connector) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

func (c *Connector) Close() error {
	return c.client.Close()
}

// toGoFieldName converts snake_case to CamelCase.
func toGoFieldName(s string) string {
	parts := strings.Split(s, "_")
	for i := range parts {
		if len(parts[i]) > 0 {
			parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
	}
	return strings.Join(parts, "")
}
