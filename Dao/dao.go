package Dao

import (
	"gorm.io/gorm"
	"qiuyier/seckill/models"
)

type daoService struct {
}

func newDaoService() *daoService {
	return &daoService{}
}

var DaoService = newDaoService()

func (r *daoService) CreateOrder(db *gorm.DB, m *models.Order) (err error) {
	err = db.Create(m).Error
	return
}

func (r *daoService) UpdateStock(db *gorm.DB, id int64) (err error) {
	err = db.Model(&models.Goods{}).Where("id = ?", id).Update("stock", gorm.Expr("stock - 1")).Error
	return
}

func (r *daoService) UpdateMqSendStatus(db *gorm.DB, id int64) (err error) {
	err = db.Model(&models.MqSend{}).Where("id = ?", id).Update("status", 1).Error
	return
}

func (r *daoService) CreateMqSend(db *gorm.DB, m *models.MqSend) (err error) {
	err = db.Create(m).Error
	return
}

func (r *daoService) CreateMqProcess(db *gorm.DB, m *models.MqProcess) (err error) {
	err = db.Create(m).Error
	return
}

func (r *daoService) FindOneByMqSendId(db *gorm.DB, out interface{}, id int64) interface{} {
	if err := db.Where("mq_send_id = ?", id).First(out).Error; err != nil {
		return nil
	}
	return out
}
