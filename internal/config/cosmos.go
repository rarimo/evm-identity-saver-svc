package config

import (
	"time"

	"gitlab.com/distributed_lab/logan/v3/errors"

	"github.com/tendermint/tendermint/rpc/client/http"
	"gitlab.com/distributed_lab/figure"
	"gitlab.com/distributed_lab/kit/kv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

func (c *config) Cosmos() *grpc.ClientConn {
	return c.cosmos.Do(func() interface{} {
		var config struct {
			Addr string `fig:"addr"`
		}

		if err := figure.Out(&config).From(kv.MustGetStringMap(c.getter, "cosmos")).Please(); err != nil {
			panic(errors.Wrap(err, "failed to figure out"))
		}

		con, err := grpc.Dial(config.Addr, grpc.WithInsecure(), grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:    10 * time.Second, // wait time before ping if no activity
			Timeout: 20 * time.Second, // ping timeout
		}))
		if err != nil {
			panic(errors.Wrap(err, "failed to dial cosmos grpc"))
		}

		return con
	}).(*grpc.ClientConn)
}

func (c *config) Tendermint() *http.HTTP {
	return c.tendermint.Do(func() interface{} {
		var config struct {
			Addr string `fig:"addr"`
		}

		if err := figure.Out(&config).From(kv.MustGetStringMap(c.getter, "core")).Please(); err != nil {
			panic(errors.Wrap(err, "failed to figure out"))
		}

		client, err := http.New(config.Addr, "/websocket")
		if err != nil {
			panic(errors.Wrap(err, "failed to create tendermint client"))
		}

		if err := client.Start(); err != nil {
			panic(errors.Wrap(err, "failed to start tendermint client"))
		}

		return client
	}).(*http.HTTP)
}
