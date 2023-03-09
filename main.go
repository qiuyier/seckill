package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/go-co-op/gocron"
	"github.com/justinas/alice"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"io"
	"math/rand"
	"net/http"
	"os"
	"qiuyier/seckill/Dao"
	"qiuyier/seckill/amqp"
	"qiuyier/seckill/models"
	"strconv"
	"time"
)

var ctx = context.Background()

// 随机拒绝，可以大量减少请求
func randomRejectHandler(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		rand.NewSource(time.Now().UnixNano())
		randomNum := rand.Intn(101)
		randomNumString := strconv.Itoa(randomNum)
		if randomNum < 80 {
			http.Error(w, http.StatusText(403)+"_"+randomNumString, 403)
			return
		}
		next.ServeHTTP(w, r)
	}
	return http.HandlerFunc(fn)
}

func recoverHandler(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				logrus.Errorf("panic: %+v", err)
				http.Error(w, http.StatusText(500), 500)
			}
		}()

		next.ServeHTTP(w, r)
	}

	return http.HandlerFunc(fn)
}

type SecKillParam struct {
	UserId string `json:"user_id"`
}

// 登录校验，模拟没有登录直接拒绝
func loginHandler(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ := io.ReadAll(r.Body)
		bodyString := []byte(bodyBytes)
		//print raw response body for debugging purposes
		r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		params := SecKillParam{}
		err := json.Unmarshal(bodyString, &params)

		if err != nil {
			http.Error(w, err.Error(), 401)
		}

		if params.UserId == "" {
			http.Error(w, http.StatusText(401), 401)
			return
		}

		next.ServeHTTP(w, r)
	}
	return http.HandlerFunc(fn)
}

// Struct2map 将struct转换成map
func Struct2map(content interface{}) (map[string]interface{}, error) {
	jsonStr, _ := json.Marshal(content)
	result := make(map[string]interface{})
	err := json.Unmarshal(jsonStr, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func init() {
	logrus.SetOutput(os.Stdout)

	//连接Redis
	if err := Dao.InitRedis(); err != nil {
		logrus.Error(err)
	}

	//连接数据库
	if err := Dao.InitDb(models.Models...); err != nil {
		logrus.Error(err)
	}
	Dao.Db().Model(&models.Goods{}).Where("id = ?", 1).Update("stock", 10)
	CacheGoodsInfo()

	// 启用job定时把丢失的mq再次处理
	scheduler := gocron.NewScheduler(time.Local)
	_, err := scheduler.Every(10).Seconds().Do(func() {
		logrus.Println("crontab working....")
		var mqSendData []*models.MqSend
		if err := Dao.Db().Where("status = ? and times < ? and is_deleted = ?", 0, 10, 0).Find(&mqSendData).Error; err != nil {
			logrus.Errorf("mqSend data search failed: %s", err.Error())
		}
		if len(mqSendData) > 0 {
			logrus.Println("mq working....")
			mq := amqp.NewRabbitMQSimple("queue_publisher")
			for _, mqSend := range mqSendData {
				var body map[string]interface{}
				err := json.Unmarshal([]byte(mqSend.Body), &body)
				if err != nil {
					continue
				}
				body["mq_send_id"] = mqSend.Id
				mq.PublishSimple(body)
				Dao.Db().Model(&models.MqSend{}).Where("id = ?", mqSend.Id).Update("times", gorm.Expr("times + 1"))
			}
			mq.Destroy()
		}
	})
	if err != nil {
		return
	}
	scheduler.StartAsync()
}

func main() {
	handlers := alice.New(randomRejectHandler, loginHandler, recoverHandler)
	http.Handle("/order/secKill", handlers.ThenFunc(SecKillHandler))
	http.HandleFunc("/goodsInfo", GetGoodsInfoHandler)
	err := http.ListenAndServe(":8000", nil)
	if err != nil {
		logrus.Error(err)
	}
}
