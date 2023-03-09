package amqp

import (
	"encoding/json"
	"fmt"
	"github.com/mitchellh/mapstructure"
	"github.com/sirupsen/logrus"
	"github.com/streadway/amqp"
	"gorm.io/gorm"
	"qiuyier/seckill/Dao"
	"qiuyier/seckill/models"
)

// MQUrl 格式 amqp://账号:密码@rabbitmq服务器地址:端口号/vhost
const MQUrl = "amqp://admin:admin@localhost:5672/"

type RabbitMQ struct {
	conn      *amqp.Connection
	channel   *amqp.Channel
	QueueName string // 队列名称
	Exchange  string // 交换机
	Key       string // Key
	MQUrl     string // 连接信息
}

// NewRabbitMQ 创建结构体实例
func NewRabbitMQ(queueName, exchange, key string) *RabbitMQ {
	rabbitmq := &RabbitMQ{
		QueueName: queueName,
		Exchange:  exchange,
		Key:       key,
		MQUrl:     MQUrl,
	}
	var err error
	rabbitmq.conn, err = amqp.Dial(rabbitmq.MQUrl)
	rabbitmq.failOnErr(err, "创建连接错误!")

	rabbitmq.channel, err = rabbitmq.conn.Channel()
	rabbitmq.failOnErr(err, "获取channel失败!")

	return rabbitmq
}

// Destroy 断开channel和connection
func (mq *RabbitMQ) Destroy() {
	_ = mq.channel.Close()
	_ = mq.conn.Close()
}

// failOnErr 错误处理函数
func (mq *RabbitMQ) failOnErr(err error, message string) {
	if err != nil {
		//log.Fatalf("%s:%s", message, err)
		panic(fmt.Sprintf("%s:%s", message, err))
	}
}

// NewRabbitMQSimple 创建简单模式下的RabbitMQ实例
func NewRabbitMQSimple(queueName string) *RabbitMQ {
	return NewRabbitMQ(queueName, "", "")
}

// PublishSimple 简单模式下生产消息
func (mq *RabbitMQ) PublishSimple(body map[string]interface{}) {
	// 1. 申请队列, 如果队列不存在会自动创建, 如果存在则跳过创建
	// 保证队列存在, 消息能发送到队列中
	_, err := mq.channel.QueueDeclare(
		mq.QueueName,
		true,  // 是否持久化
		false, // 是否为自动删除
		false, // 是否具有排他性
		false, // 是否阻塞
		nil,   // 额外属性
	)
	if err != nil {
		logrus.Fatalf("%s:%s", "申请队列失败", err.Error())
	}

	// 2. 发送消息到队列中
	bodyJson, err := json.Marshal(body)
	if err != nil {
		logrus.Errorf("body 转换失败: %s", err.Error())
	}
	err = mq.channel.Publish(
		mq.Exchange,
		mq.QueueName,
		false, // 如果为true, 会根据exchange类型和routekey规则，如果无法找到符合条件的队列那么会把发送的消息返回给发送者
		false, // 如果为true, 当exchange发送消息到队列后发现队列上没有绑定消费者，则会把消息发还给发送者
		amqp.Publishing{
			ContentType: "text/plain",
			Body:        bodyJson,
		})
	if err != nil {
		logrus.Errorf("mq 发布失败: %s", err.Error())
	}

}

func (mq *RabbitMQ) ConsumeLoop(db *gorm.DB) {
	// 1. 申请队列, 如果队列不存在会自动创建, 如果存在则跳过创建
	// 保证队列存在, 消息能发送到队列中
	_, err := mq.channel.QueueDeclare(
		mq.QueueName,
		true,  // 是否持久化
		false, // 是否为自动删除
		false, // 是否具有排他性
		false, // 是否阻塞
		nil,   // 额外属性
	)
	if err != nil {
		logrus.Fatalf("%s:%s", "申请队列失败", err.Error())
	}

	// 2. 接收消息
	msgs, err := mq.channel.Consume(
		mq.QueueName,
		"",    // 用来区分多个消费者
		false, // 是否自动应答
		false, // 是否具有排他性
		false, // 如果设置为true，表示不能将同一个connection中发送的消息传递给这个connection中的消费者
		false, // 队列消费是否阻塞
		nil,
	)
	if err != nil {
		logrus.Fatalf("%s:%s", "申请队列失败", err.Error())
	}

	forever := make(chan bool)
	go func() {
		for {
			select {
			case msg := <-msgs:
				// 这里写你的处理逻辑
				// 获取到的消息是amqp.Delivery对象，从中可以获取消息信息
				logrus.Printf("body: %s", string(msg.Body))
				params := make(map[string]interface{})
				err = json.Unmarshal(msg.Body, &params)
				if err != nil {
					logrus.Printf("json unmarshal error: %s", err.Error())
					_ = msg.Ack(false)
					continue
				}

				orderParams := &models.Order{}

				err = mapstructure.WeakDecode(params, orderParams)
				if err != nil {
					logrus.Errorf("转换失败: %s", err.Error())
					_ = msg.Ack(false)
					continue
				}

				mqProcessParams := &models.MqProcess{}
				err = mapstructure.WeakDecode(params, mqProcessParams)
				if err != nil {
					logrus.Errorf("转换失败: %s", err.Error())
					_ = msg.Ack(false)
					continue
				}

				// 判断是否处理了，已处理则忽略
				res := &models.MqProcess{}
				mqProcessInfo := Dao.DaoService.FindOneByMqSendId(db, res, int64(mqProcessParams.MqSendId))
				if mqProcessInfo != nil {
					logrus.Println("消息已处理")
					_ = msg.Ack(true)
					continue
				}

				err = db.Transaction(func(tx *gorm.DB) error {
					if err := Dao.DaoService.CreateMqProcess(tx, mqProcessParams); err != nil {
						logrus.Errorf("记录消息处理表失败: %s", err.Error())
						return err
					}
					if err := Dao.DaoService.CreateOrder(tx, orderParams); err != nil {
						logrus.Errorf("创建订单失败: %s", err.Error())
						return err
					}

					if err := Dao.DaoService.UpdateStock(tx, int64(orderParams.GoodsId)); err != nil {
						logrus.Errorf("库存减少失败: %s", err.Error())
						return err
					}

					if err := Dao.DaoService.UpdateMqSendStatus(tx, int64(mqProcessParams.MqSendId)); err != nil {
						logrus.Errorf("消息发送表状态修改失败: %s", err.Error())
						return err
					}
					return nil
				})
				if err != nil {
					_ = msg.Ack(false)
					continue
				}
				_ = msg.Ack(true)
			}
		}
	}()
	logrus.Printf("[*] Waiting for message, To exit press CTRL+C")
	<-forever
}
