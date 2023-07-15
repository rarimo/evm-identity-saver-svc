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
	"gitlab.com/rarimo/savers/saver-grpc-lib/metrics"
)

func RunStateChangeCatchup(ctx context.Context, cfg config.Config) {
	const runnerName = "state_change_catchup"

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
		log:         log,
		chainID:     cfg.Ethereum().ChainID,
		networkName: cfg.Ethereum().NetworkName,
		broadcaster: cfg.Broadcaster(),
		handler:     handler,
		msger: rarimo.NewStateUpdateMessageMaker(
			cfg.Broadcaster().Sender(),
			cfg.Ethereum().ContractAddr.String(),
			cfg.Ethereum().NetworkName,
			stateData,
			cfg.States().IssuerID,
		),
		watchedIssuerID: cfg.States().IssuerID,
	}

	if err := listener.catchup(ctx); err != nil {
		log.WithError(err).Error("failed to process last state")
	}
}

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
		log:         log,
		chainID:     cfg.Ethereum().ChainID,
		networkName: cfg.Ethereum().NetworkName,
		broadcaster: cfg.Broadcaster(),
		handler:     handler,
		msger: rarimo.NewStateUpdateMessageMaker(
			cfg.Broadcaster().Sender(),
			cfg.Ethereum().ContractAddr.String(),
			cfg.Ethereum().NetworkName,
			stateData,
			cfg.States().IssuerID,
		),
		watchedIssuerID: cfg.States().IssuerID,
	}

	running.WithBackOff(ctx, log, runnerName,
		listener.subscription,
		5*time.Second, 5*time.Second, 5*time.Second)
}

type stateUpdateMsger interface {
	LastStateUpdateMsg(ctx context.Context) (*oracletypes.MsgCreateIdentityDefaultTransferOp, error)
}

type stateChangeListener struct {
	log         *logan.Entry
	chainID     *big.Int
	networkName string
	handler     *statebind.StateLib
	broadcaster broadcaster.Broadcaster
	msger       stateUpdateMsger

	watchedIssuerID *big.Int
}

func (l *stateChangeListener) catchup(ctx context.Context) error {
	l.log.Info("Starting state catchup")
	defer l.log.Info("Catchup finished")

	// Getting last state message
	msg, err := l.msger.LastStateUpdateMsg(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to craft last state msg")
	}

	l.log.Debug(msg)

	if err := l.broadcaster.BroadcastTx(ctx, msg); err != nil {
		return errors.Wrap(err, "failed to broadcast last state msg")
	}

	return nil
}

func (l *stateChangeListener) subscription(ctx context.Context) error {
	l.log.Info("Starting subscription")
	defer l.log.Info("Subscription finished")

	sink := make(chan *statebind.StateLibStateUpdated, 10) // TODO use actual contract
	defer close(sink)

	sub, err := l.handler.WatchStateUpdated(&bind.WatchOpts{
		Context: ctx,
	}, sink)
	if err != nil {
		return errors.Wrap(err, "failed to init state changed subscription")
	}
	defer sub.Unsubscribe()

subscriptionPoll:
	for {
		select {
		case <-ctx.Done():
			break subscriptionPoll
		case err := <-sub.Err():
			l.log.WithError(err).Error("subscription error")
			metrics.WebsocketMetric.Set(metrics.WebsocketDisconnected)
			break subscriptionPoll
		case e := <-sink:
			if e == nil {
				l.log.Error("got nil event")
				continue
			}

			l.log.WithFields(logan.F{
				"tx_hash":   e.Raw.TxHash,
				"tx_index":  e.Raw.TxIndex,
				"log_index": e.Raw.Index,
			}).Debug("got event")

			if e.Id.Cmp(l.watchedIssuerID) != 0 {
				continue
			}

			// Getting last state message
			msg, err := l.msger.LastStateUpdateMsg(ctx)
			if err != nil {
				return errors.Wrap(err, "failed to craft state updated msg", logan.F{
					"tx_hash": e.Raw.TxHash.String(),
				})
			}

			l.log.Debug(msg)

			if err := l.broadcaster.BroadcastTx(ctx, msg); err != nil {
				return errors.Wrap(err, "failed to broadcast state updated msg")
			}
		}
	}

	return nil
}
