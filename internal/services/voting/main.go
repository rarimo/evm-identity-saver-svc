package voting

import (
	"context"
	"sync"
	"time"

	"github.com/rarimo/evm-identity-saver-svc/internal/config"
	rarimocore "github.com/rarimo/rarimo-core/x/rarimocore/types"
	"github.com/rarimo/saver-grpc-lib/voter"
	"gitlab.com/distributed_lab/running"
)

const (
	OpQueryGISTUpdate  = "tm.event='Tx' AND new_operation.operation_type='IDENTITY_GIST_TRANSFER'"
	OpQueryStateUpdate = "tm.event='Tx' AND new_operation.operation_type='IDENTITY_STATE_TRANSFER'"
)

func RunVoter(ctx context.Context, cfg config.Config) {
	gistV := NewGISTUpdateVerifier(cfg)
	stateV := NewStateUpdateVerifier(cfg)

	v := voter.NewVoter(cfg.Ethereum().NetworkName, cfg.Log(), cfg.Broadcaster(), map[rarimocore.OpType]voter.Verifier{
		rarimocore.OpType_IDENTITY_STATE_TRANSFER: stateV,
		rarimocore.OpType_IDENTITY_GIST_TRANSFER:  gistV,
	})

	// catchup tends to panic on startup and doesn't handle it by itself, so we wrap it into retry loop
	running.UntilSuccess(ctx, cfg.Log(), "voter-catchup", func(ctx context.Context) (bool, error) {
		voter.
			NewCatchupper(cfg.Cosmos(), v, cfg.Log()).
			Run(ctx)

		return true, nil
	}, 1*time.Second, 5*time.Second)

	wg := sync.WaitGroup{}
	wg.Add(2)

	go func() {
		defer wg.Done()
		voter.
			NewSubscriber(v, cfg.Tendermint(), cfg.Cosmos(), OpQueryStateUpdate, cfg.Log(), cfg.Subscriber()).
			Run(ctx)
	}()

	go func() {
		defer wg.Done()
		voter.
			NewSubscriber(v, cfg.Tendermint(), cfg.Cosmos(), OpQueryGISTUpdate, cfg.Log(), cfg.Subscriber()).
			Run(ctx)
	}()

	wg.Wait()
}
