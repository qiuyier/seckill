package main

import (
	"github.com/sirupsen/logrus"
	"os"
	"qiuyier/seckill/Dao"
	"qiuyier/seckill/amqp"
	"qiuyier/seckill/models"
)

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
}

func main() {
	//mq := amqp.NewRabbitMQ("queue_publisher", "exchange_publisher", "order")
	mq := amqp.NewRabbitMQSimple("queue_publisher")
	//mq.ConsumeSimple(Dao.Db())
	mq.ConsumeLoop(Dao.Db())
}
