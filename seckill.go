package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"github.com/linvon/cuckoo-filter"
	"github.com/mitchellh/mapstructure"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"io"
	"net/http"
	"qiuyier/seckill/Dao"
	"qiuyier/seckill/amqp"
	"qiuyier/seckill/models"
	"time"
)

var (
	//go:embed script/secKill.lua
	lua      string
	cacheKey string = "goods:goods_1"
	lockKey  string = "lock_get_goods_1"
	stockKey string = "stock:goods_1"
	goodsId  string = "1"
	cf              = cuckoo.NewFilter(4, 9, 3900, cuckoo.TableTypePacked)
	// 初始化mq
	//mq = amqp.NewRabbitMQ("queue_publisher", "exchange_publisher", "order")
	mq = amqp.NewRabbitMQSimple("queue_publisher")
)

func SecKillHandler(w http.ResponseWriter, r *http.Request) {
	rdb := Dao.Rdb()
	db := Dao.Db()

	body := r.Body
	all, _ := io.ReadAll(body)
	params := SecKillParam{}
	err := json.Unmarshal(all, &params)

	if err != nil {
		ResponseHandle(w, 0, "json:"+err.Error(), nil, http.StatusOK)
		return
	}

	//使用lua脚本预扣除库存，若返回1则秒杀成功，进入下一步，反之失败
	res, err := rdb.Eval(ctx, lua, []string{stockKey, "order:user_id_" + params.UserId}).Int()
	if err != nil {
		ResponseHandle(w, 0, "lua: "+err.Error(), nil, http.StatusOK)
		return
	}

	if res == 1 {
		// 通过rabbitmq生成订单
		mqBody := map[string]interface{}{
			"user_id":  params.UserId,
			"goods_id": goodsId,
		}
		bodyJson, err := json.Marshal(mqBody)
		if err != nil {
			logrus.Errorf("body 转换失败: %s", err.Error())
			ResponseHandle(w, 0, "已购完", nil, http.StatusOK)
			return
		}

		// 记录数据到消息发送表
		err = db.Transaction(func(tx *gorm.DB) error {
			mqSend := &models.MqSend{
				Body:   string(bodyJson),
				Status: 0,
				Times:  1,
			}
			if err := Dao.DaoService.CreateMqSend(tx, mqSend); err != nil {
				return err
			}
			mqBody["mq_send_id"] = mqSend.Id

			mq.PublishSimple(mqBody)
			return nil
		})
		if err != nil {
			logrus.Errorf("生成订单失败: %s", err.Error())
			ResponseHandle(w, 0, "已购完", nil, http.StatusOK)
		}

	} else if res == 2 {
		//判断是否重复下单
		result, err := rdb.Incr(ctx, "order:user_id_"+params.UserId).Result()
		if err != nil {
			ResponseHandle(w, 0, "redis: "+err.Error(), nil, http.StatusOK)
			return
		}
		if result > 1 {
			ResponseHandle(w, 0, "您已提交了订单，请勿重复提交", nil, http.StatusOK)
			return
		}
	} else {
		ResponseHandle(w, 0, "已购完", nil, http.StatusOK)
		return
	}
	ResponseHandle(w, 1, "success", nil, http.StatusOK)
}

func ResponseHandle(w http.ResponseWriter, code int, msg string, data interface{}, httpCode int) {
	response := map[string]interface{}{
		"code": code,
		"msg":  msg,
		"data": data,
	}
	jsonResp, _ := json.MarshalIndent(response, "", "")
	if httpCode != http.StatusOK {
		w.WriteHeader(httpCode)
	}
	_, err := w.Write(jsonResp)
	if err != nil {
		return
	}
}

// CacheGoodsInfo 第一步 预热商品信息
func CacheGoodsInfo() {
	rdb := Dao.Rdb()
	db := Dao.Db()
	goodsInfo := &models.Goods{}
	if err := db.Take(goodsInfo, "id = ?", 1).Error; err != nil {
		logrus.Errorf("get goods info failed: %s", err.Error())
	}
	goodsInfoMap, _ := Struct2map(goodsInfo)

	err := rdb.HSet(ctx, cacheKey, goodsInfoMap).Err()
	if err != nil {
		logrus.Errorf("set goods info cache failed: %s", err.Error())
	}

	// 设置缓存过期
	//err = rdb.Expire(ctx, cacheKey, time.Second*time.Duration(86400)).Err()
	//if err != nil {
	//	logrus.Errorf("set cache ttl failed: %s", err.Error())
	//}

	// 单独缓存库存，以便做预扣库存
	err = rdb.Set(ctx, stockKey, goodsInfo.Stock, 0).Err()
	if err != nil {
		logrus.Errorf("set goods stock cache failed: %s", err.Error())
	}
	// 数据缓存到布谷过滤器
	go func() {
		cf.Add([]byte(goodsId))
	}()
}

func GetGoodsInfoHandler(w http.ResponseWriter, r *http.Request) {
	info, err := GetGoodsInfo()
	if err != nil {
		ResponseHandle(w, 0, err.Error(), nil, http.StatusOK)
		return
	}

	ResponseHandle(w, 1, "success", info, http.StatusOK)
}

// GetGoodsInfo 获取商品信息
func GetGoodsInfo() (*models.Goods, error) {
	rdb := Dao.Rdb()
	db := Dao.Db()
	// 先判断缓存有没有
	// 缓存有直接缓存获取
	// 缓存没有则通过分布式锁获取信息缓存，避免缓存击穿

	// 数据量少可以用redis的set，节省空间，数据量大可以使用bloom filter
	if cf.Contain([]byte(goodsId)) {
		goodsInfo := &models.Goods{}
		c := NewClient(rdb)
		res, _ := c.client.HGetAll(ctx, cacheKey).Result()
		if len(res) <= 0 {
			// 获取锁
			lock, err := c.TryLock(ctx, lockKey, time.Second)
			if err != nil {
				return nil, err
			}

			// 获取锁成功,重新读取缓存
			if lock.key == lockKey {
				res, err = c.client.HGetAll(ctx, cacheKey).Result()
				if len(res) <= 0 {
					//没有数据则从数据库读取
					if err = db.Take(goodsInfo, "id = ?", 1).Error; err != nil {
						// 释放锁
						_ = lock.Unlock(ctx, cacheKey)
						return nil, err
					}
					goodsInfoMap, _ := Struct2map(goodsInfo)
					err = rdb.HSet(ctx, cacheKey, goodsInfoMap).Err()
					if err != nil {
						// 释放锁
						_ = lock.Unlock(ctx, cacheKey)
						return nil, err
					}
				}

				// 释放锁
				err = lock.Unlock(ctx, cacheKey)
				if err != nil {
					return nil, err
				}
			}
		} else {
			// map转structure
			err := mapstructure.WeakDecode(res, goodsInfo)
			if err != nil {
				return nil, err
			}
		}

		return goodsInfo, nil
	} else {
		return nil, errors.New("查无此商品")
	}
}
