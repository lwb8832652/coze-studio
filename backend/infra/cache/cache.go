/*
 * Copyright 2025 coze-dev Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package cache

import (
	"context"
	"errors"
	"time"
)

var (
	Nil                     error
	ErrReadinessUnavailable = errors.New("cache readiness unavailable")
)

func SetDefaultNilError(err error) {
	Nil = err
}

type Cmdable interface {
	Pipeline() Pipeliner
	StringCmdable
	HashCmdable
	GenericCmdable
	ListCmdable
	ScriptCmdable
}

// ReadinessChecker performs a bounded, side-effect-free connectivity check.
// Provider control-plane wiring requires this capability and never falls back
// to command-interface presence as a readiness signal.
type ReadinessChecker interface {
	CheckReadiness(context.Context) error
}

// ScriptCmdable executes one server-side script through the shared cache
// connection. Implementations should use the script SHA and transparently
// fall back to EVAL when Redis has not loaded it yet.
type ScriptCmdable interface {
	RunScript(ctx context.Context, script string, keys []string, args ...interface{}) ScriptCmd
}

type StringCmdable interface {
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) StatusCmd
	Get(ctx context.Context, key string) StringCmd
	IncrBy(ctx context.Context, key string, value int64) IntCmd
	Incr(ctx context.Context, key string) IntCmd
}

type HashCmdable interface {
	HSet(ctx context.Context, key string, values ...interface{}) IntCmd
	HGetAll(ctx context.Context, key string) MapStringStringCmd
}

type GenericCmdable interface {
	Del(ctx context.Context, keys ...string) IntCmd
	Exists(ctx context.Context, keys ...string) IntCmd
	Expire(ctx context.Context, key string, expiration time.Duration) BoolCmd
}

type Pipeliner interface {
	StatefulCmdable
	Exec(ctx context.Context) ([]Cmder, error)
}

type StatefulCmdable interface {
	Cmdable
}

type ListCmdable interface {
	LIndex(ctx context.Context, key string, index int64) StringCmd
	LPush(ctx context.Context, key string, values ...interface{}) IntCmd
	RPush(ctx context.Context, key string, values ...interface{}) IntCmd
	LSet(ctx context.Context, key string, index int64, value interface{}) StatusCmd
	LPop(ctx context.Context, key string) StringCmd
	LRange(ctx context.Context, key string, start, stop int64) StringSliceCmd
}
type Cmder interface {
	Err() error
}

type baseCmd interface {
	Err() error
}

type IntCmd interface {
	baseCmd
	Result() (int64, error)
}

type MapStringStringCmd interface {
	baseCmd
	Result() (map[string]string, error)
}

type BoolCmd interface {
	baseCmd
	Result() (bool, error)
}

type StatusCmd interface {
	baseCmd
	Result() (string, error)
}

type StringCmd interface {
	baseCmd
	Result() (string, error)
	Val() string
	Int64() (int64, error)
	Bytes() ([]byte, error)
}

type StringSliceCmd interface {
	baseCmd
	Result() ([]string, error)
}

type ScriptCmd interface {
	baseCmd
	Result() (interface{}, error)
}
