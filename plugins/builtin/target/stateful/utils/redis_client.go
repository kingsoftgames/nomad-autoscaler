package utils

import (
	"github.com/gomodule/redigo/redis"
	"github.com/hashicorp/go-hclog"
	"sync"
	"time"
)

var globalRedisPool *redis.Pool
var globalRedisPoolOnce sync.Once

var redisAddress string
var redisPassword string
var maxIdleConn int
var maxActiveConn int

func GetRedis() redis.Conn {
	globalRedisPoolOnce.Do(func() {
		globalRedisPool = NewRedisPool()
	})
	return globalRedisPool.Get()
}

type RedisConfig struct {
	Address       string `json:"address"`
	Password      string `json:"password"`
	MaxActiveConn int    `json:"max_active_conn"`
	MaxIdleConn   int    `json:"max_idle_conn"`
}

func StartRedisService(redisConfig RedisConfig) {
	redisAddress = redisConfig.Address
	redisPassword = redisConfig.Password
	maxActiveConn = redisConfig.MaxActiveConn
	maxIdleConn = redisConfig.MaxIdleConn
}

// 仅供测试用
func StartRedisService2(address, password string) {
	redisAddress = address
	redisPassword = password
	maxActiveConn = 1000
	maxIdleConn = 5000
}

func NewRedisPool(log hclog.Logger) *redis.Pool {
	return &redis.Pool{
		MaxIdle:     maxIdleConn,
		MaxActive:   maxActiveConn,
		IdleTimeout: 300 * time.Second,
		Dial: func() (redis.Conn, error) {
			c, err := redis.Dial("tcp", redisAddress)
			if err != nil {
				log.Trace("redis Dial", "connect redis server fail", "err", err, "address", redisAddress, "password", redisPassword)
				return nil, err
			}
			if len(redisPassword) > 0 {
				if _, err := c.Do("AUTH", redisPassword); err != nil {
					c.Close()
					log.Trace("redis Dial", "connect redis server fail", "err", err, "address", redisAddress, "password", redisPassword)
					return nil, err
				}
			}
			log.Trace("redis Dial", "connect redis server", "address", redisAddress, "password", redisPassword)
			return c, err
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			if time.Since(t) < 2*time.Minute {
				return nil
			}
			_, err := c.Do("PING")
			return err
		},
	}
}

func InnerDo(conn redis.Conn, commandName string, args ...interface{}) (reply interface{}, err error) {
	reply, err = conn.Do(commandName, args...)
	return
}

func InnerSend(conn redis.Conn, commandName string, args ...interface{}) (err error) {
	err = conn.Send(commandName, args...)
	if err == nil {
		err = conn.Flush()
	}
	return
}

func String(reply interface{}, err error) (string, error) {
	return redis.String(reply, err)
}
func Byte(reply interface{}, err error) ([]byte, error) {
	ret, err := redis.String(reply, err)
	if err != nil && err.Error() == "redigo: nil returned" {
		err = nil
	}
	return []byte(ret), err
}

func Bool(reply interface{}, err error) (bool, error) {
	return redis.Bool(reply, err)

}

func Int(reply interface{}, err error) (int, error) {
	return redis.Int(reply, err)
}

func Int64(reply interface{}, err error) (int64, error) {
	return redis.Int64(reply, err)
}

func RedisGet(key string) (reply interface{}, err error) {
	conn := GetRedis()
	reply, err = InnerDo(conn, "GET", key)
	conn.Close()
	return
}

func RedisSet(key string, value interface{}) (err error) {
	conn := GetRedis()
	_, err = InnerDo(conn, "SET", key, value)
	conn.Close()
	return
}

func RedisSendSet(key string, value interface{}) (err error) {
	conn := GetRedis()
	err = InnerSend(conn, "SET", key, value)
	conn.Close()
	return
}

func RedisDel(key string) (err error) {
	conn := GetRedis()
	_, err = InnerDo(conn, "DEL", key)
	conn.Close()
	return
}

func RedisINCR(key string) (incrReply int64, err error) {
	conn := GetRedis()
	reply, err := InnerDo(conn, "INCR", key)

	incrReply, err = Int64(reply, err)

	conn.Close()
	return
}

func RedisHGETALL(key string) (hashesReply map[string]string, err error) {
	conn := GetRedis()
	reply, err := InnerDo(conn, "HGETALL", key)

	hashesReply, err = redis.StringMap(reply, err)

	conn.Close()
	return
}
