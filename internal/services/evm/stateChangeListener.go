package evm

import (
	"context"
	"math/big"
	"time"

	"gitlab.com/distributed_lab/logan/v3"
	"gitlab.com/rarimo/polygonid/evm-identity-saver-svc/internal/rarimo"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"gitlab.com/distributed_lab/logan/v3/errors"
	"gitlab.com/distributed_lab/running"
	"gitlab.com/rarimo/polygonid/evm-identity-saver-svc/internal/config"
	statebind "gitlab.com/rarimo/polygonid/evm-identity-saver-svc/pkg/state"
	oracletypes "gitlab.com/rarimo/rarimo-core/x/oraclemanager/types"
	"gitlab.com/rarimo/savers/saver-grpc-lib/broadcaster"
)

const MaxBlocksPerRequest = 100

func RunStateChangeListener(ctx context.Context, cfg config.Config) {
	const runnerName = "state_change_listener"

	log := cfg.Log().WithField("who", runnerName)

	handler, err := statebind.NewStateLib(cfg.Ethereum().ContractAddr, cfg.Ethereum().RPCClient)
	if err != nil {
		panic(errors.Wrap(err, "failed to init state change handler"))
	}

	stateData, err := statebind.NewStateV2Handler(cfg.Ethereum().ContractAddr, cfg.Ethereum().RPCClient)
	if err != nil {
		panic(errors.Wrap(err, "failed to init state change handler"))
	}

	listener := stateChangeListener{
		log:          log,
		broadcaster:  cfg.Broadcaster(),
		handler:      handler,
		blockHandler: cfg.Ethereum().RPCClient,
		msger: rarimo.NewStateUpdateMessageMaker(
			cfg.Broadcaster().Sender(),
			cfg.Ethereum().ContractAddr.String(),
			cfg.Ethereum().NetworkName,
			stateData,
			cfg.States().IssuerID,
		),
		watchedIssuerID: cfg.States().IssuerID,
		fromBlock:       cfg.Ethereum().StartFromBlock,
		blockWindow:     cfg.Ethereum().BlockWindow,
	}

	running.WithBackOff(ctx, log, runnerName,
		listener.subscription,
		30*time.Second, 5*time.Second, 30*time.Second)
}

type stateUpdateMsger interface {
	StateUpdateMsgByBlock(ctx context.Context, block *big.Int) (*oracletypes.MsgCreateIdentityDefaultTransferOp, error)
}

type blockHandler interface {
	BlockNumber(ctx context.Context) (uint64, error)
}

type stateChangeListener struct {
	log          *logan.Entry
	handler      *statebind.StateLib
	broadcaster  broadcaster.Broadcaster
	msger        stateUpdateMsger
	blockHandler blockHandler

	watchedIssuerID *big.Int
	fromBlock       uint64
	blockWindow     uint64
}

func (l *stateChangeListener) subscription(ctx context.Context) error {
	lastBlock, err := l.blockHandler.BlockNumber(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get recent block")
	}

	lastBlock -= l.blockWindow

	if lastBlock < l.fromBlock {
		l.log.Infof("Skipping window: start %d > finish %d", l.fromBlock, lastBlock)
		return nil
	}

	if l.fromBlock+MaxBlocksPerRequest < lastBlock {
		l.log.Debugf("maxBlockPerRequest limit exceeded: setting last block to %d instead of %d", l.fromBlock+MaxBlocksPerRequest, lastBlock)
		lastBlock = l.fromBlock + MaxBlocksPerRequest
	}

	l.log.Infof("Starting subscription from %d to %d", l.fromBlock, lastBlock)
	defer l.log.Info("Subscription finished")

	const chanelBufSize = 10
	sink := make(chan *statebind.StateLibStateUpdated, chanelBufSize)
	defer close(sink)

	iter, err := l.handler.FilterStateUpdated(&bind.FilterOpts{
		Start:   l.fromBlock,
		End:     &lastBlock,
		Context: ctx,
	})

	if err != nil {
		return errors.Wrap(err, "failed to filter state update events")
	}

	defer func() {
		// https://ethereum.stackexchange.com/questions/8199/are-both-the-eth-newfilter-from-to-fields-inclusive
		// End in FilterLogs is inclusive
		l.fromBlock = lastBlock + 1
	}()

	for iter.Next() {
		e := iter.Event

		if e == nil {
			l.log.Error("got nil event")
			continue
		}

		l.log.WithFields(logan.F{
			"tx_hash":   e.Raw.TxHash,
			"tx_index":  e.Raw.TxIndex,
			"log_index": e.Raw.Index,
		}).Debugf("got event: id: %s block: %s timestamp: %s state: %s", e.Id.String(), e.BlockN.String(), e.Timestamp.String(), e.State.String())

		if e.Id.Cmp(l.watchedIssuerID) != 0 {
			l.log.Debugf("Skipping event: other issuer, required: %s, got: %s", l.watchedIssuerID.String(), e.Id.String())
			continue
		}

		// Getting last state message
		msg, err := l.msger.StateUpdateMsgByBlock(ctx, e.BlockN)
		if err != nil {
			l.log.WithError(err).WithField("tx_hash", e.Raw.TxHash.String()).Error("failed to craft state updated msg")
			continue
		}

		if err := l.broadcaster.BroadcastTx(ctx, msg); err != nil {
			l.log.WithError(err).WithField("tx_hash", e.Raw.TxHash.String()).Error(err, "failed to broadcast state updated msg")
			continue
		}
	}

	return nil
}
