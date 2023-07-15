package config

import (
	"github.com/tendermint/tendermint/rpc/client/http"
	"gitlab.com/distributed_lab/kit/comfig"
	"gitlab.com/distributed_lab/kit/kv"
	"gitlab.com/rarimo/savers/saver-grpc-lib/broadcaster"
	"gitlab.com/rarimo/savers/saver-grpc-lib/metrics"
	"gitlab.com/rarimo/savers/saver-grpc-lib/voter"
	"google.golang.org/grpc"
)

type Config interface {
	comfig.Logger
	comfig.Listenerer
	broadcaster.Broadcasterer
	voter.Subscriberer
	metrics.Profilerer

	Ethereum() *Ethereum
	Cosmos() *grpc.ClientConn
	Tendermint() *http.HTTP
	States() StateV2Config
}

type config struct {
	comfig.Logger
	comfig.Listenerer
	broadcaster.Broadcasterer
	voter.Subscriberer
	metrics.Profilerer

	ethereum   comfig.Once
	cosmos     comfig.Once
	tendermint comfig.Once
	states     comfig.Once
	redis      comfig.Once

	getter kv.Getter
}

func New(getter kv.Getter) Config {
	return &config{
		getter:        getter,
		Logger:        comfig.NewLogger(getter, comfig.LoggerOpts{}),
		Listenerer:    comfig.NewListenerer(getter),
		Broadcasterer: broadcaster.New(getter),
		Subscriberer:  voter.NewSubscriberer(getter),
		Profilerer:    metrics.New(getter),
	}
}
