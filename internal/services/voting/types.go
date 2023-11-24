package voting

import (
	"context"

	oracletypes "github.com/rarimo/rarimo-core/x/oraclemanager/types"
)

type stateUpdateMsger interface {
	StateUpdateMsgByHashes(ctx context.Context, issuer, latestStateHash, replacedStateHash string) (*oracletypes.MsgCreateIdentityStateTransferOp, error)
	GISTUpdateMsgByHashes(ctx context.Context, latestGISTHash, replacedGISTHash string) (*oracletypes.MsgCreateIdentityGISTTransferOp, error)
}
