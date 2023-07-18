package voting

import (
	"context"
	"time"

	"github.com/gogo/protobuf/proto"
	"gitlab.com/distributed_lab/logan/v3"
	"gitlab.com/distributed_lab/logan/v3/errors"
	"gitlab.com/distributed_lab/running"
	"gitlab.com/rarimo/polygonid/evm-identity-saver-svc/internal/config"
	"gitlab.com/rarimo/polygonid/evm-identity-saver-svc/internal/rarimo"
	statebind "gitlab.com/rarimo/polygonid/evm-identity-saver-svc/pkg/state"
	oracletypes "gitlab.com/rarimo/rarimo-core/x/oraclemanager/types"
	rarimocore "gitlab.com/rarimo/rarimo-core/x/rarimocore/types"
	"gitlab.com/rarimo/savers/saver-grpc-lib/voter"
	"gitlab.com/rarimo/savers/saver-grpc-lib/voter/verifiers"
)

const (
	OpQueryStateUpdate = "tm.event='Tx' AND new_operation.operation_type='IDENTITY_DEFAULT_TRANSFER'"
)

type stateUpdateMsger interface {
	StateUpdateMsgByHashes(ctx context.Context, latestStateHash, replacedStateHash, latestGISTHash, replacedGISTHash string) (*oracletypes.MsgCreateIdentityDefaultTransferOp, error)
}

type stateUpdateVerifier struct {
	log               *logan.Entry
	homeChain         string
	oracleQueryClient oracletypes.QueryClient
	msger             stateUpdateMsger
}

func NewStateUpdateVerifier(cfg config.Config) *stateUpdateVerifier {
	stateV2, err := statebind.NewStateV2Handler(cfg.Ethereum().ContractAddr, cfg.Ethereum().RPCClient)
	if err != nil {
		panic(errors.Wrap(err, "failed to init StateV2 filterer"))
	}

	return &stateUpdateVerifier{
		log:               cfg.Log(),
		homeChain:         cfg.Ethereum().NetworkName,
		oracleQueryClient: oracletypes.NewQueryClient(cfg.Cosmos()),
		msger: rarimo.NewStateUpdateMessageMaker(
			cfg.Broadcaster().Sender(),
			cfg.Ethereum().ContractAddr.String(),
			cfg.Ethereum().NetworkName,
			stateV2,
			cfg.States().IssuerID,
		),
	}
}

func RunStateUpdateVoter(ctx context.Context, cfg config.Config) {
	verifier := NewStateUpdateVerifier(cfg)

	v := voter.NewVoter(cfg.Ethereum().NetworkName, cfg.Log(), cfg.Broadcaster(), map[rarimocore.OpType]voter.Verifier{
		rarimocore.OpType_IDENTITY_DEFAULT_TRANSFER: verifier,
	})

	// catchup tends to panic on startup and doesn't handle it by itself, so we wrap it into retry loop
	running.UntilSuccess(ctx, cfg.Log(), "state-update-voter-catchup", func(ctx context.Context) (bool, error) {
		voter.
			NewCatchupper(cfg.Cosmos(), v, cfg.Log()).
			Run(ctx)

		return true, nil
	}, 1*time.Second, 5*time.Second)

	// run blocking verification subscription
	voter.
		NewSubscriber(v, cfg.Tendermint(), cfg.Cosmos(), OpQueryStateUpdate, cfg.Log(), cfg.Subscriber()).
		Run(ctx)
}

func (s *stateUpdateVerifier) Verify(ctx context.Context, operation rarimocore.Operation) (rarimocore.VoteType, error) {
	if operation.OperationType != rarimocore.OpType_IDENTITY_DEFAULT_TRANSFER {
		s.log.Debugf("Voted NO: invalid operation type")
		return rarimocore.VoteType_NO, verifiers.ErrInvalidOperationType
	}

	var stateUpdated rarimocore.IdentityDefaultTransfer
	if err := proto.Unmarshal(operation.Details.Value, &stateUpdated); err != nil {
		s.log.Debugf("Voted NO: failed to unmarshal")
		return rarimocore.VoteType_NO, err
	}

	if err := s.verifyIdentityDefaultTransfer(ctx, stateUpdated); err != nil {
		s.log.WithError(err).Debugf("Voted NO: received an error from verifier")
		switch errors.Cause(err) {
		case verifiers.ErrUnsupportedNetwork:
			return rarimocore.VoteType_NO, verifiers.ErrUnsupportedNetwork
		case verifiers.ErrWrongOperationContent:
			return rarimocore.VoteType_NO, nil
		default:
			return rarimocore.VoteType_NO, err
		}
	}

	return rarimocore.VoteType_YES, nil

}

func (s *stateUpdateVerifier) verifyIdentityDefaultTransfer(ctx context.Context, transfer rarimocore.IdentityDefaultTransfer) error {
	msg, err := s.msger.StateUpdateMsgByHashes(ctx, transfer.StateHash, transfer.ReplacedStateHash, transfer.GISTHash, transfer.ReplacedGISTHash)
	if err != nil {
		return errors.Wrap(err, "failed to get msg")
	}

	resp, err := s.oracleQueryClient.IdentityDefaultTransfer(ctx, &oracletypes.QueryGetIdentityDefaultTransferRequest{Msg: *msg})
	if err != nil {
		return errors.Wrap(err, "failed to fetch operation details from core")
	}

	if !proto.Equal(&resp.Transfer, &transfer) {
		return verifiers.ErrWrongOperationContent
	}

	return nil
}
