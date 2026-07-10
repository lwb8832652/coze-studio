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

package idgen

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/coze-dev/coze-studio/backend/infra/cache"

	"github.com/coze-dev/coze-studio/backend/infra/idgen"
)

const (
	counterKeyExpirationTime = 10 * time.Minute
	maxCounterPosition       = 255
	debugLocalServerID       = 1
)

type IDGenerator = idgen.IDGenerator

var debugLocalIDGen = struct {
	sync.Mutex
	lastMs  int64
	counter int64
}{}

func New(client cache.Cmdable) (idgen.IDGenerator, error) {
	// Initialization code.
	return &idGenImpl{
		cli: client,
	}, nil
}

type idGenImpl struct {
	cli       cache.Cmdable
	namespace string
}

func (i *idGenImpl) GenID(ctx context.Context) (int64, error) {
	ids, err := i.GenMultiIDs(ctx, 1)
	if err != nil {
		return 0, err
	}

	return ids[0], nil
}

func (i *idGenImpl) GenMultiIDs(ctx context.Context, counts int) ([]int64, error) {
	const maxTimeAddrTimes = 8

	leftNum := int64(counts)
	lastMs := int64(0)
	ids := make([]int64, 0, counts)
	svrID := int64(0) // A server id is all 0.

	for idx := int64(0); leftNum > 0 && idx < maxTimeAddrTimes; idx++ {
		ms := maxInt64(i.GetIDTimeMs(), lastMs)
		if ms <= lastMs {
			ms++
		}
		lastMs = ms

		redisKey := genIDKey(i.namespace, svrID, ms)

		counterPosition, err := i.IncrBy(ctx, redisKey, leftNum)
		if err != nil {
			if debugIDGenFallbackEnabled() {
				return genDebugLocalIDs(counts), nil
			}
			return nil, err
		}

		var start, end int64
		start = counterPosition - leftNum

		if start == 0 {
			i.Expire(ctx, redisKey)
		}

		if start > maxCounterPosition {
			continue
		} else if counterPosition < leftNum {
			return nil, fmt.Errorf("recycling of counting space occurs, ms=%v", ms)
		}

		if counterPosition > maxCounterPosition {
			end = maxCounterPosition + 1
			leftNum = counterPosition - maxCounterPosition - 1
		} else {
			end = counterPosition
			leftNum = 0
		}

		seconds := ms / 1000
		millis := ms % 1000

		if seconds&0xFFFFFFFF != seconds {
			return nil, fmt.Errorf("seconds more than 32 bits, seconds=%v", seconds)
		}

		if svrID&0x3FFF != svrID {
			return nil, fmt.Errorf("server id more than 14 bits, serverID=%v", svrID)
		}

		for i := start; i < end; i++ {
			// fmt.Printf("sec=%v, ms=%v, counter=%v\n", seconds, millis, i)
			id := (seconds)<<32 + (millis)<<22 + i<<14 + svrID
			ids = append(ids, id)
		}
	}

	if len(ids) < counts || leftNum != 0 {
		return nil, fmt.Errorf("IDs num not enough, ns=%v, expect=%v, gotten=%v, lastMs=%v", i.namespace, counts, len(ids), lastMs)
	}

	return ids, nil
}

func (i *idGenImpl) IncrBy(ctx context.Context, key string, num int64) (cntPos int64, err error) {
	return i.cli.IncrBy(ctx, key, num).Result()
}

func (i *idGenImpl) GetIDTimeMs() int64 {
	return time.Now().UnixNano() / int64(time.Millisecond)
}

func (i *idGenImpl) Expire(ctx context.Context, key string) {
	// Temporarily ignore errors
	_, _ = i.cli.Expire(ctx, key, counterKeyExpirationTime).Result()
}

func genIDKey(space string, svrID int64, ms int64) string {
	// Once the format of this key is determined, it cannot be changed
	return fmt.Sprintf("id_generator:%v:%v:%v", space, svrID, ms)
}

func debugIDGenFallbackEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("COZE_DEBUG_IDGEN_FALLBACK"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func genDebugLocalIDs(counts int) []int64 {
	debugLocalIDGen.Lock()
	defer debugLocalIDGen.Unlock()

	ids := make([]int64, 0, counts)
	for len(ids) < counts {
		ms := time.Now().UnixNano() / int64(time.Millisecond)
		if ms != debugLocalIDGen.lastMs {
			debugLocalIDGen.lastMs = ms
			debugLocalIDGen.counter = 0
		}
		if debugLocalIDGen.counter > maxCounterPosition {
			time.Sleep(time.Millisecond)
			continue
		}

		seconds := ms / 1000
		millis := ms % 1000
		id := (seconds << 32) + (millis << 22) + (debugLocalIDGen.counter << 14) + debugLocalServerID
		ids = append(ids, id)
		debugLocalIDGen.counter++
	}

	return ids
}

func maxInt64(a, b int64) int64 {
	if a <= b {
		return b
	} else {
		return a
	}
}
