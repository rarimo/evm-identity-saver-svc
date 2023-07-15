package config

import (
	"math/big"

	"gitlab.com/distributed_lab/figure"
	"gitlab.com/distributed_lab/kit/kv"
	"gitlab.com/distributed_lab/logan/v3/errors"
)

type StateV2Config struct {
	IssuerID *big.Int `fig:"issuer_id,required"`
}

func (c *config) States() StateV2Config {
	return c.states.Do(func() interface{} {
		const serviceName = "state_contract_cfg"

		var cfg StateV2Config

		err := figure.Out(&cfg).From(kv.MustGetStringMap(c.getter, serviceName)).Please()
		if err != nil {
			panic(errors.Wrap(err, "failed to figure out "+serviceName))
		}

		return cfg
	}).(StateV2Config)
}
