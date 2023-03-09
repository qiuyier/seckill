package models

var Models = []interface{}{
	&Goods{}, &Order{}, &MqSend{}, &MqProcess{},
}

// DefaultModel 每张表固定有的字段
type DefaultModel struct {
	Id         int   `gorm:"primaryKey;autoIncrement;type:int(1)" json:"id" form:"id" mapstructure:"id"`
	IsDeleted  int   `gorm:"type:tinyint(1);default:0;comment:是否删除,1-已删除 0-未删除;" json:"is_deleted" form:"is_deleted" mapstructure:"is_deleted"`
	CreateTime int64 `gorm:"not null;type:int(1);autoCreateTime;comment:创建时间;" json:"create_time" form:"create_time" mapstructure:"create_time"`
	UpdateTime int64 `gorm:"not null;type:int(1);autoUpdateTime;comment:更新时间;" json:"update_time" form:"update_time" mapstructure:"update_time"`
}

type Goods struct {
	DefaultModel `mapstructure:",squash"`
	GoodsName    string  `gorm:"size:50;not null;comment:商品名;" json:"goods_name" form:"goods_name" mapstructure:"goods_name"`
	GoodsPrize   float64 `gorm:"type:decimal(10,2);not null;comment:商品价格;" json:"goods_prize" form:"goods_prize" mapstructure:"goods_prize"`
	Stock        int     `gorm:"type:int(1);not null;default:0;comment:商品库存" json:"stock" form:"stock" mapstructure:"stock"`
}

type Order struct {
	DefaultModel
	GoodsId int `gorm:"type:int(1);not null;comment:商品ID;" json:"goods_id" form:"goods_id" mapstructure:"goods_id"`
	UserId  int `gorm:"type:int(1);not null;comment:用户ID;" json:"user_id" form:"user_id" mapstructure:"user_id"`
	Status  int `gorm:"type:tinyint(1);not null;comment:-1-取消 0-待支付 1-已支付;default:0;" json:"status" form:"status" mapstructure:"status"`
}

type MqSend struct {
	DefaultModel
	Body   string `gorm:"type:text;not null;comment:数据包;" json:"body" form:"body" mapstructure:"body"`
	Status int    `gorm:"type:tinyint(1);not null;comment:0-未处理 1-已处理;default:0;" json:"status" form:"status" mapstructure:"status"`
	Times  int    `gorm:"type:tinyint(1);not null;comment:发送次数;default:0;" json:"times" form:"times" mapstructure:"times"`
}

type MqProcess struct {
	DefaultModel
	MqSendId int `gorm:"type:tinyint(1);not null;comment:消息发送表ID;" json:"mq_send_id" form:"mq_send_id" mapstructure:"mq_send_id"`
}
