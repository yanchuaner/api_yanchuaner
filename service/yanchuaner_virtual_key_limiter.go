/*
Copyright (C) 2026 Yanchuaner Ecosystem Contributors

This file is original Yanchuaner Ecosystem work and is distributed under
the GNU Affero General Public License version 3 or later with this repository.
*/
package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

const yanCoreVirtualKeyReservationContextKey = "yancore_virtual_key_rate_reservation"

var (
	ErrYanCoreVirtualKeyRPMExceeded         = errors.New("virtual key RPM limit exceeded")
	ErrYanCoreVirtualKeyTPMExceeded         = errors.New("virtual key TPM limit exceeded")
	ErrYanCoreVirtualKeyConcurrencyExceeded = errors.New("virtual key concurrency limit exceeded")
	ErrYanCoreVirtualKeyLimiterUnavailable  = errors.New("virtual key limiter backend unavailable")
)

const yanCoreVirtualKeyAcquireScript = `
local rpm_limit = tonumber(ARGV[1])
local reserved_tokens = tonumber(ARGV[2])
local tpm_limit = tonumber(ARGV[3])
local concurrency_limit = tonumber(ARGV[4])
local rpm_applied = 0
local tpm_applied = 0

if rpm_limit > 0 then
  local rpm = redis.call('INCR', KEYS[1])
  redis.call('EXPIRE', KEYS[1], 120)
  rpm_applied = 1
  if rpm > rpm_limit then
    redis.call('DECR', KEYS[1])
    return 1
  end
end

if tpm_limit > 0 and reserved_tokens > 0 then
  local tpm = redis.call('INCRBY', KEYS[2], reserved_tokens)
  redis.call('EXPIRE', KEYS[2], 120)
  tpm_applied = 1
  if tpm > tpm_limit then
    if rpm_applied == 1 then redis.call('DECR', KEYS[1]) end
    redis.call('DECRBY', KEYS[2], reserved_tokens)
    return 2
  end
end

if concurrency_limit > 0 then
  local active = redis.call('INCR', KEYS[3])
  redis.call('EXPIRE', KEYS[3], 3600)
  if active > concurrency_limit then
    redis.call('DECR', KEYS[3])
    if rpm_applied == 1 then redis.call('DECR', KEYS[1]) end
    if tpm_applied == 1 then redis.call('DECRBY', KEYS[2], reserved_tokens) end
    return 3
  end
end

return 0
`

const yanCoreVirtualKeyReleaseScript = `
local release_concurrency = tonumber(ARGV[1])
local release_tokens = tonumber(ARGV[2])
if release_concurrency == 1 then
  local active = redis.call('DECR', KEYS[1])
  if active <= 0 then redis.call('DEL', KEYS[1]) end
end
if release_tokens > 0 then
  local tokens = redis.call('DECRBY', KEYS[2], release_tokens)
  if tokens <= 0 then redis.call('DEL', KEYS[2]) end
end
return 0
`

const yanCoreVirtualKeyAdjustTPMScript = `
local delta = tonumber(ARGV[1])
local tokens = redis.call('INCRBY', KEYS[1], delta)
redis.call('EXPIRE', KEYS[1], 120)
if tokens <= 0 then redis.call('DEL', KEYS[1]) end
return tokens
`

type yanCoreVirtualKeyUsageWindow struct {
	rpm int
	tpm int64
}

type yanCoreVirtualKeyWindowKey struct {
	tokenID int
	minute  int64
}

var yanCoreVirtualKeyMemoryLimiter = struct {
	sync.Mutex
	windows    map[yanCoreVirtualKeyWindowKey]*yanCoreVirtualKeyUsageWindow
	concurrent map[int]int
}{windows: map[yanCoreVirtualKeyWindowKey]*yanCoreVirtualKeyUsageWindow{}, concurrent: map[int]int{}}

type yanCoreVirtualKeyReservation struct {
	sync.Mutex
	tokenID            int
	windowKey          yanCoreVirtualKeyWindowKey
	redisTPMKey        string
	redisConcurrentKey string
	reservedTokens     int64
	usesRedis          bool
	concurrencyHeld    bool
	recorded           bool
	released           bool
}

func yanCoreVirtualKeyLimiterContext(c *gin.Context) context.Context {
	if c != nil && c.Request != nil {
		return c.Request.Context()
	}
	return context.Background()
}

func BeginYanCoreVirtualKeyRequest(c *gin.Context, estimatedTokens int) error {
	policy := YanCoreVirtualKeyPolicyFromContext(c)
	if policy == nil {
		return nil
	}
	if policy.Status != model.YanCoreVirtualKeyPolicyActive || estimatedTokens < 0 {
		return model.ErrYanCoreVirtualKeyPolicyInvalid
	}
	if current, ok := c.Get(yanCoreVirtualKeyReservationContextKey); ok && current != nil {
		return nil
	}
	minute := time.Now().Unix() / 60
	windowKey := yanCoreVirtualKeyWindowKey{tokenID: policy.TokenId, minute: minute}
	reservation := &yanCoreVirtualKeyReservation{tokenID: policy.TokenId, windowKey: windowKey, reservedTokens: int64(estimatedTokens), concurrencyHeld: policy.MaxConcurrency > 0}

	if common.RedisEnabled {
		if common.RDB == nil {
			return ErrYanCoreVirtualKeyLimiterUnavailable
		}
		redisWindowKey := fmt.Sprintf("%d:%d", policy.TokenId, minute)
		rpmKey := "yancore:vkey:rpm:" + redisWindowKey
		tpmKey := "yancore:vkey:tpm:" + redisWindowKey
		concurrencyKey := fmt.Sprintf("yancore:vkey:concurrency:%d", policy.TokenId)
		result, err := common.RDB.Eval(yanCoreVirtualKeyLimiterContext(c), yanCoreVirtualKeyAcquireScript, []string{rpmKey, tpmKey, concurrencyKey}, policy.MaxRPM, estimatedTokens, policy.MaxTPM, policy.MaxConcurrency).Int()
		if err != nil {
			return fmt.Errorf("%w: %v", ErrYanCoreVirtualKeyLimiterUnavailable, err)
		}
		switch result {
		case 1:
			return ErrYanCoreVirtualKeyRPMExceeded
		case 2:
			return ErrYanCoreVirtualKeyTPMExceeded
		case 3:
			return ErrYanCoreVirtualKeyConcurrencyExceeded
		}
		reservation.usesRedis = true
		reservation.redisTPMKey = tpmKey
		reservation.redisConcurrentKey = concurrencyKey
	} else {
		yanCoreVirtualKeyMemoryLimiter.Lock()
		for key := range yanCoreVirtualKeyMemoryLimiter.windows {
			if key.minute < minute-1 {
				delete(yanCoreVirtualKeyMemoryLimiter.windows, key)
			}
		}
		window := yanCoreVirtualKeyMemoryLimiter.windows[windowKey]
		if window == nil {
			window = &yanCoreVirtualKeyUsageWindow{}
			yanCoreVirtualKeyMemoryLimiter.windows[windowKey] = window
		}
		if policy.MaxRPM > 0 && window.rpm+1 > policy.MaxRPM {
			yanCoreVirtualKeyMemoryLimiter.Unlock()
			return ErrYanCoreVirtualKeyRPMExceeded
		}
		if policy.MaxTPM > 0 && window.tpm+int64(estimatedTokens) > int64(policy.MaxTPM) {
			yanCoreVirtualKeyMemoryLimiter.Unlock()
			return ErrYanCoreVirtualKeyTPMExceeded
		}
		if policy.MaxConcurrency > 0 && yanCoreVirtualKeyMemoryLimiter.concurrent[policy.TokenId]+1 > policy.MaxConcurrency {
			yanCoreVirtualKeyMemoryLimiter.Unlock()
			return ErrYanCoreVirtualKeyConcurrencyExceeded
		}
		window.rpm++
		window.tpm += int64(estimatedTokens)
		if policy.MaxConcurrency > 0 {
			yanCoreVirtualKeyMemoryLimiter.concurrent[policy.TokenId]++
		}
		yanCoreVirtualKeyMemoryLimiter.Unlock()
	}
	c.Set(yanCoreVirtualKeyReservationContextKey, reservation)
	return nil
}

func RecordYanCoreVirtualKeyTokens(c *gin.Context, actualTokens int) error {
	value, ok := c.Get(yanCoreVirtualKeyReservationContextKey)
	if !ok {
		return nil
	}
	reservation, ok := value.(*yanCoreVirtualKeyReservation)
	if !ok || reservation == nil || actualTokens < 0 {
		return model.ErrYanCoreVirtualKeyPolicyInvalid
	}
	reservation.Lock()
	defer reservation.Unlock()
	if reservation.recorded || reservation.released {
		return nil
	}
	delta := int64(actualTokens) - reservation.reservedTokens
	if reservation.usesRedis {
		if common.RDB == nil {
			return ErrYanCoreVirtualKeyLimiterUnavailable
		}
		_, err := common.RDB.Eval(yanCoreVirtualKeyLimiterContext(c), yanCoreVirtualKeyAdjustTPMScript, []string{reservation.redisTPMKey}, delta).Result()
		reservation.recorded = true
		if err != nil {
			return fmt.Errorf("%w: %v", ErrYanCoreVirtualKeyLimiterUnavailable, err)
		}
	} else {
		yanCoreVirtualKeyMemoryLimiter.Lock()
		if window := yanCoreVirtualKeyMemoryLimiter.windows[reservation.windowKey]; window != nil {
			window.tpm += delta
			if window.tpm < 0 {
				window.tpm = 0
			}
		}
		yanCoreVirtualKeyMemoryLimiter.Unlock()
		reservation.recorded = true
	}
	return nil
}

func FinalizeYanCoreVirtualKeyRequest(c *gin.Context) error {
	value, ok := c.Get(yanCoreVirtualKeyReservationContextKey)
	if !ok {
		return nil
	}
	reservation, ok := value.(*yanCoreVirtualKeyReservation)
	if !ok || reservation == nil {
		return nil
	}
	reservation.Lock()
	defer reservation.Unlock()
	if reservation.released {
		return nil
	}
	releaseTokens := int64(0)
	if !reservation.recorded {
		releaseTokens = reservation.reservedTokens
	}
	if reservation.usesRedis {
		if common.RDB == nil {
			return ErrYanCoreVirtualKeyLimiterUnavailable
		}
		releaseConcurrency := 0
		if reservation.concurrencyHeld {
			releaseConcurrency = 1
		}
		_, err := common.RDB.Eval(yanCoreVirtualKeyLimiterContext(c), yanCoreVirtualKeyReleaseScript, []string{reservation.redisConcurrentKey, reservation.redisTPMKey}, releaseConcurrency, releaseTokens).Result()
		reservation.released = true
		if err != nil {
			return fmt.Errorf("%w: %v", ErrYanCoreVirtualKeyLimiterUnavailable, err)
		}
	} else {
		yanCoreVirtualKeyMemoryLimiter.Lock()
		if releaseTokens > 0 {
			if window := yanCoreVirtualKeyMemoryLimiter.windows[reservation.windowKey]; window != nil {
				window.tpm -= releaseTokens
				if window.tpm < 0 {
					window.tpm = 0
				}
			}
		}
		if reservation.concurrencyHeld && yanCoreVirtualKeyMemoryLimiter.concurrent[reservation.tokenID] > 0 {
			yanCoreVirtualKeyMemoryLimiter.concurrent[reservation.tokenID]--
		}
		yanCoreVirtualKeyMemoryLimiter.Unlock()
		reservation.released = true
	}
	return nil
}

func resetYanCoreVirtualKeyLimiterForTest() {
	yanCoreVirtualKeyMemoryLimiter.Lock()
	yanCoreVirtualKeyMemoryLimiter.windows = map[yanCoreVirtualKeyWindowKey]*yanCoreVirtualKeyUsageWindow{}
	yanCoreVirtualKeyMemoryLimiter.concurrent = map[int]int{}
	yanCoreVirtualKeyMemoryLimiter.Unlock()
}
