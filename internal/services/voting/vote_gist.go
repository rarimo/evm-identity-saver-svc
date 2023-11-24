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

type GistUpdateVerifier struct {
	log               *logan.Entry
	oracleQueryClient oracletypes.QueryClient
	msger             stateUpdateMsger
}

var _ voter.Verifier = &GistUpdateVerifier{}

func NewGISTUpdateVerifier(cfg config.Config) *GistUpdateVerifier {
	stateV2, err := statebind.NewStateV2Handler(cfg.Ethereum().ContractAddr, cfg.Ethereum().RPCClient)
	if err != nil {
		panic(errors.Wrap(err, "failed to init StateV2 filterer"))
	}

	return &GistUpdateVerifier{
		log:               cfg.Log(),
		oracleQueryClient: oracletypes.NewQueryClient(cfg.Cosmos()),
		msger: rarimo.NewStateUpdateMessageMaker(
			cfg.Broadcaster().Sender(),
			cfg.Ethereum().ContractAddr.String(),
			cfg.Ethereum().NetworkName,
			stateV2,
		),
	}
}

func (s *GistUpdateVerifier) Verify(ctx context.Context, operation rarimocore.Operation) (rarimocore.VoteType, error) {
	if operation.OperationType != rarimocore.OpType_IDENTITY_GIST_TRANSFER {
		s.log.Debugf("Voted NO: invalid operation type")
		return rarimocore.VoteType_NO, verifiers.ErrInvalidOperationType
	}

	var op rarimocore.IdentityGISTTransfer
	if err := proto.Unmarshal(operation.Details.Value, &op); err != nil {
		s.log.Debugf("Voted NO: failed to unmarshal")
		return rarimocore.VoteType_NO, err
	}

	if err := s.verifyIdentityGISTTransfer(ctx, op); err != nil {
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

func (s *GistUpdateVerifier) verifyIdentityGISTTransfer(ctx context.Context, op rarimocore.IdentityGISTTransfer) error {
	msg, err := s.msger.GISTUpdateMsgByHashes(ctx, op.GISTHash, op.ReplacedGISTHash)
	if err != nil {
		return errors.Wrap(err, "failed to get msg")
	}

	resp, err := s.oracleQueryClient.IdentityGISTTransfer(ctx, &oracletypes.QueryGetIdentityGISTTransferRequest{Msg: *msg})
	if err != nil {
		return errors.Wrap(err, "failed to fetch operation details from core")
	}

	if !proto.Equal(&resp.Transfer, &op) {
		return verifiers.ErrWrongOperationContent
	}

	return nil
}
