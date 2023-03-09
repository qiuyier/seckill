package Dao

import (
	"context"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
)

var rdb redis.Cmdable

func InitRedis() (err error) {
	rdb = redis.NewClient(&redis.Options{
		Addr: "127.0.0.1:6379",
	})

	if err = rdb.Ping(context.Background()).Err(); err != nil {
		logrus.Errorf("redis connect failed: %s", err.Error())
	}
	return
}

func Rdb() redis.Cmdable {
	return rdb
}
