package main

import (
	"context"
	_ "embed"
	"errors"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"time"
)

var (
	//go:embed script/unlock.lua
	luaUnlock              string
	ErrFailedToPreemptLock = errors.New("抢锁失败")
	ErrLockNotHold         = errors.New("未持有锁")
)

type Client struct {
	client redis.Cmdable
}

func NewClient(c redis.Cmdable) *Client {
	return &Client{
		client: c,
	}
}

func (c *Client) TryLock(ctx context.Context, key string, expiration time.Duration) (*Lock, error) {
	value := uuid.New().String()
	res, err := c.client.SetNX(ctx, key, value, expiration).Result()
	if err != nil {
		return nil, err
	}
	if !res {
		return nil, ErrFailedToPreemptLock
	}
	return newLock(c.client, key, value), nil
}

type Lock struct {
	client redis.Cmdable
	key    string
	value  string
}

func newLock(client redis.Cmdable, key, value string) *Lock {
	return &Lock{
		client: client,
		key:    key,
		value:  value,
	}
}

func (l *Lock) Unlock(ctx context.Context, key string) error {
	res, err := l.client.Eval(ctx, luaUnlock, []string{l.key}, l.value).Int64()
	if err == redis.Nil {
		return ErrLockNotHold
	}
	if err != nil {
		return err
	}
	if res != 1 {
		return ErrLockNotHold
	}
	return nil
}
