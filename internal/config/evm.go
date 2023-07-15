package config

import (
	"context"
	"math/big"
	"reflect"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/spf13/cast"
	"gitlab.com/distributed_lab/figure"
	"gitlab.com/distributed_lab/kit/kv"
	"gitlab.com/distributed_lab/logan/v3/errors"
)

type Ethereum struct {
	ContractAddr   common.Address    `fig:"contract_addr,required"`
	RPCClient      *ethclient.Client `fig:"rpc,required"`
	StartFromBlock uint64            `fig:"start_from_block"`
	NetworkName    string            `fig:"network_name,required"`
	ChainID        *big.Int          `fig:"-"`
}

func (c *config) Ethereum() *Ethereum {
	return c.ethereum.Do(func() interface{} {
		cfg := Ethereum{}

		err := figure.
			Out(&cfg).
			With(figure.BaseHooks, evmHooks).
			From(kv.MustGetStringMap(c.getter, "evm")).
			Please()
		if err != nil {
			panic(errors.Wrap(err, "failed to figure out evm config"))
		}

		cID, err := cfg.RPCClient.ChainID(context.TODO())
		if err != nil {
			panic(errors.Wrap(err, "failed to get chain id"))
		}

		cfg.ChainID = cID

		return &cfg
	}).(*Ethereum)
}

var evmHooks = figure.Hooks{
	"common.Address": func(raw interface{}) (reflect.Value, error) {
		v, err := cast.ToStringE(raw)
		if err != nil {
			return reflect.Value{}, errors.Wrap(err, "expected string")
		}

		return reflect.ValueOf(common.HexToAddress(v)), nil
	},
	"*ethclient.Client": func(raw interface{}) (reflect.Value, error) {
		v, err := cast.ToStringE(raw)
		if err != nil {
			return reflect.Value{}, errors.Wrap(err, "expected string")
		}

		client, err := ethclient.Dial(v)
		if err != nil {
			return reflect.Value{}, errors.Wrap(err, "failed to dial eth rpc")
		}

		return reflect.ValueOf(client), nil
	},
}
