package Dao

import (
	"database/sql"
	"github.com/sirupsen/logrus"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

var (
	db    *gorm.DB
	sqlDb *sql.DB
)

func InitDb(models ...interface{}) (err error) {
	config := &gorm.Config{}

	config.NamingStrategy = schema.NamingStrategy{
		TablePrefix:   "mall_",
		SingularTable: true,
	}

	//config.Logger = logger.New(log.New(os.Stdout, "\r\n", log.LstdFlags), logger.Config{
	//	SlowThreshold: time.Second,
	//	Colorful:      true,
	//	LogLevel:      logger.Info,
	//})

	dsn := "root:123456@tcp(127.0.0.1:3307)/mall?charset=utf8mb4&parseTime=True&loc=Local"

	if db, err = gorm.Open(mysql.Open(dsn), config); err != nil {
		logrus.Errorf("database connect failed: %s", err.Error())
		return
	}

	if sqlDb, err = db.DB(); err == nil {
		sqlDb.SetMaxIdleConns(50)
		sqlDb.SetMaxOpenConns(200)
	} else {
		logrus.Error(err)
	}

	if err = db.AutoMigrate(models...); err != nil {
		logrus.Errorf("auto migrate failed: %s", err.Error())
	}

	return
}

func Db() *gorm.DB {
	return db
}
