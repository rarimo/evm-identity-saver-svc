package config

import (
	redis "github.com/go-redis/redis/v8"
	"gitlab.com/distributed_lab/figure"
	"gitlab.com/distributed_lab/kit/kv"
	"gitlab.com/distributed_lab/logan/v3/errors"
)

type Redis struct {
	Addr     string `fig:"addr,required"`
	Password string `fig:"password"`
}

func (c *config) Redis() *redis.Client {
	return c.redis.Do(func() interface{} {
		const serviceName = "redis"

		var cfg Redis

		err := figure.Out(&cfg).From(kv.MustGetStringMap(c.getter, serviceName)).Please()
		if err != nil {
			panic(errors.Wrap(err, "failed to figure out "+serviceName))
		}

		return redis.NewClient(&redis.Options{Addr: cfg.Addr, Password: cfg.Password})
	}).(*redis.Client)
}
