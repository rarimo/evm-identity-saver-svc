package config

import (
	"gitlab.com/distributed_lab/figure"
	"gitlab.com/distributed_lab/kit/kv"
	"gitlab.com/distributed_lab/logan/v3/errors"
)

type StateV2Config struct {
	IssuerID            []string `fig:"issuer_id"`
	DisableFiltration   bool     `fig:"disable_filtration"`
	StatesPerRequest    int64    `fig:"states_per_request"`
	MaxBlocksPerRequest uint64   `fig:"max_blocks_per_request"`
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
