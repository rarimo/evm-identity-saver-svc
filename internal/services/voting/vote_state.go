package voting

import (
	"context"

	"github.com/gogo/protobuf/proto"
	"github.com/rarimo/evm-identity-saver-svc/internal/config"
	"github.com/rarimo/evm-identity-saver-svc/internal/rarimo"
	statebind "github.com/rarimo/evm-identity-saver-svc/pkg/state"
	oracletypes "github.com/rarimo/rarimo-core/x/oraclemanager/types"
	rarimocore "github.com/rarimo/rarimo-core/x/rarimocore/types"
	"github.com/rarimo/saver-grpc-lib/voter"
	"github.com/rarimo/saver-grpc-lib/voter/verifiers"
	"gitlab.com/distributed_lab/logan/v3"
	"gitlab.com/distributed_lab/logan/v3/errors"
)

type StateUpdateVerifier struct {
	log               *logan.Entry
	oracleQueryClient oracletypes.QueryClient
	msger             stateUpdateMsger
}

var _ voter.Verifier = &StateUpdateVerifier{}

func NewStateUpdateVerifier(cfg config.Config) *StateUpdateVerifier {
	stateV2, err := statebind.NewStateV2Handler(cfg.Ethereum().ContractAddr, cfg.Ethereum().RPCClient)
	if err != nil {
		panic(errors.Wrap(err, "failed to init StateV2 filterer"))
	}

	return &StateUpdateVerifier{
		log:               cfg.Log(),
		oracleQueryClient: oracletypes.NewQueryClient(cfg.Cosmos()),
		msger: rarimo.NewStateUpdateMessageMaker(
			cfg.Broadcaster().Sender(),
			cfg.Ethereum().ContractAddr.String(),
			cfg.Ethereum().NetworkName,
			cfg.States().StatesPerRequest,
			stateV2,
		),
	}
}

func (s *StateUpdateVerifier) Verify(ctx context.Context, operation rarimocore.Operation) (rarimocore.VoteType, error) {
	if operation.OperationType != rarimocore.OpType_IDENTITY_STATE_TRANSFER {
		s.log.Debugf("Voted NO: invalid operation type")
		return rarimocore.VoteType_NO, verifiers.ErrInvalidOperationType
	}

	var op rarimocore.IdentityStateTransfer
	if err := proto.Unmarshal(operation.Details.Value, &op); err != nil {
		s.log.Debugf("Voted NO: failed to unmarshal")
		return rarimocore.VoteType_NO, err
	}

	if err := s.verifyIdentityStateTransfer(ctx, op); err != nil {
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

func (s *StateUpdateVerifier) verifyIdentityStateTransfer(ctx context.Context, transfer rarimocore.IdentityStateTransfer) error {
	msg, err := s.msger.StateUpdateMsgByHashes(ctx, transfer.Id, transfer.StateHash, transfer.ReplacedStateHash)
	if err != nil {
		return errors.Wrap(err, "failed to get msg")
	}

	resp, err := s.oracleQueryClient.IdentityStateTransfer(ctx, &oracletypes.QueryGetIdentityStateTransferRequest{Msg: *msg})
	if err != nil {
		return errors.Wrap(err, "failed to fetch operation details from core")
	}

	if !proto.Equal(&resp.Transfer, &transfer) {
		return verifiers.ErrWrongOperationContent
	}

	return nil
}
