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

package coze

import (
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

const (
	canonicalProductDefaultLimit int32 = 50
	canonicalProductMaximumLimit int32 = 200
)

// canonicalProductPage keeps exact offset semantics for product resources.
// Application DTOs consume Page and Limit while public responses retain Offset.
type canonicalProductPage struct {
	Limit  int32
	Offset int32
	Page   int32
}

func canonicalProductPagination(c *app.RequestContext) (canonicalProductPage, *canonicalError) {
	limit, public := canonicalProductPageValue(c, "limit", canonicalProductDefaultLimit)
	if public != nil {
		return canonicalProductPage{}, public
	}
	if limit > canonicalProductMaximumLimit {
		limit = canonicalProductMaximumLimit
	}
	offset, public := canonicalProductPageValue(c, "offset", 0)
	if public != nil {
		return canonicalProductPage{}, public
	}
	if offset%limit != 0 {
		return canonicalProductPage{}, canonicalProductPaginationError("offset")
	}
	page := int64(offset)/int64(limit) + 1
	if page > int64(^uint32(0)>>1) {
		return canonicalProductPage{}, canonicalProductPaginationError("offset")
	}
	return canonicalProductPage{Limit: limit, Offset: offset, Page: int32(page)}, nil
}

func canonicalProductPageValue(c *app.RequestContext, name string, defaultValue int32) (int32, *canonicalError) {
	if c == nil {
		return defaultValue, nil
	}
	raw, exists := c.GetQuery(name)
	if !exists {
		return defaultValue, nil
	}
	if raw == "" || strings.TrimSpace(raw) != raw {
		return 0, canonicalProductPaginationError(name)
	}
	for _, character := range raw {
		if character < '0' || character > '9' {
			return 0, canonicalProductPaginationError(name)
		}
	}
	value, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || value < 0 || (name == "limit" && value == 0) {
		return 0, canonicalProductPaginationError(name)
	}
	return int32(value), nil
}

func canonicalProductPaginationError(name string) *canonicalError {
	return newCanonicalError(
		consts.StatusUnprocessableEntity,
		"invalid_pagination",
		"Invalid query parameter: "+name,
		"invalid_pagination",
		false,
	)
}

func (p canonicalProductPage) hasMore(total int64) bool {
	return int64(p.Offset)+int64(p.Limit) < total
}

func (p canonicalProductPage) nextCursor(total int64) *string {
	if !p.hasMore(total) {
		return nil
	}
	next := strconv.FormatInt(int64(p.Offset)+int64(p.Limit), 10)
	return &next
}

// canonicalProductPathID makes product handlers share the canonical decimal
// path parser instead of accepting a second spelling of resource IDs.
func canonicalProductPathID(c *app.RequestContext, name string) (int64, *canonicalError) {
	return canonicalPathID(c, name)
}

// decodeCanonicalProductJSON composes the canonical body ceiling with the
// strict single-object decoder used by the core Thread and Run handlers.
func decodeCanonicalProductJSON(c *app.RequestContext, dst any) *canonicalError {
	if public := canonicalRequestBodyLimit(c, "Product"); public != nil {
		return public
	}
	return decodeCanonicalJSON(c, dst)
}
