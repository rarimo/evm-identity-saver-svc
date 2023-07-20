package rarimo

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"

	"gitlab.com/distributed_lab/logan/v3/errors"
	statebind "gitlab.com/rarimo/polygonid/evm-identity-saver-svc/pkg/state"
	oracletypes "gitlab.com/rarimo/rarimo-core/x/oraclemanager/types"
)

type StateDataProvider interface {
	GetStateInfoByIdAndState(opts *bind.CallOpts, id *big.Int, state *big.Int) (statebind.IStateStateInfo, error)
	GetGISTRootInfo(opts *bind.CallOpts, root *big.Int) (statebind.IStateGistRootInfo, error)

	GetStateInfoHistoryLengthById(opts *bind.CallOpts, id *big.Int) (*big.Int, error)
	GetStateInfoHistoryById(opts *bind.CallOpts, id *big.Int, startIndex *big.Int, length *big.Int) ([]statebind.IStateStateInfo, error)

	GetGISTRootHistory(opts *bind.CallOpts, start *big.Int, length *big.Int) ([]statebind.IStateGistRootInfo, error)
	GetGISTRootHistoryLength(opts *bind.CallOpts) (*big.Int, error)
}

type StateUpdateMessageMaker struct {
	txCreatorAddr     string
	contract          string
	homeChain         string
	stateDataProvider StateDataProvider
	watchedIssuerID   *big.Int
}

func NewStateUpdateMessageMaker(txCreatorAddr string, contract string, homeChain string, stateDataProvider StateDataProvider, issuerID *big.Int) *StateUpdateMessageMaker {
	return &StateUpdateMessageMaker{txCreatorAddr: txCreatorAddr, contract: contract, homeChain: homeChain, stateDataProvider: stateDataProvider, watchedIssuerID: issuerID}
}

func (m *StateUpdateMessageMaker) StateUpdateMsgByBlock(ctx context.Context, block *big.Int) (*oracletypes.MsgCreateIdentityDefaultTransferOp, error) {
	latestState, replacedState, err := m.getStatesOnBlock(ctx, block)

	length, err := m.stateDataProvider.GetGISTRootHistoryLength(&bind.CallOpts{
		Context: ctx,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get overall gists count")
	}

	// FIXME: Possible bug: GIST could be changed during our logic processing
	// Can be done using the same logic as with states
	gists, err := m.stateDataProvider.GetGISTRootHistory(&bind.CallOpts{
		Context: ctx,
	}, new(big.Int).Sub(length, big.NewInt(2)), big.NewInt(2))

	if err != nil {
		return nil, errors.Wrap(err, "failed to get last two gist")
	}

	replacedGIST := gists[0]
	latestGIST := gists[1]

	return m.StateUpdateMsgByStates(ctx, *latestState, *replacedState, latestGIST, replacedGIST)
}

func (m *StateUpdateMessageMaker) StateUpdateMsgByHashes(ctx context.Context, latestStateHash, replacedStateHash, latestGISTHash, replacedGISTHash string) (*oracletypes.MsgCreateIdentityDefaultTransferOp, error) {
	latestState, err := m.stateDataProvider.GetStateInfoByIdAndState(&bind.CallOpts{
		Context: ctx,
	}, m.watchedIssuerID, new(big.Int).SetBytes(hexutil.MustDecode(latestStateHash)))
	if err != nil {
		return nil, errors.Wrap(err, "failed to get latest state")
	}

	replacedState, err := m.stateDataProvider.GetStateInfoByIdAndState(&bind.CallOpts{
		Context: ctx,
	}, m.watchedIssuerID, new(big.Int).SetBytes(hexutil.MustDecode(replacedStateHash)))
	if err != nil {
		return nil, errors.Wrap(err, "failed to get replaced state")
	}

	latestGIST, err := m.stateDataProvider.GetGISTRootInfo(&bind.CallOpts{
		Context: ctx,
	}, new(big.Int).SetBytes(hexutil.MustDecode(latestGISTHash)))
	if err != nil {
		return nil, errors.Wrap(err, "failed to get latest gist")
	}

	replacedGIST, err := m.stateDataProvider.GetGISTRootInfo(&bind.CallOpts{
		Context: ctx,
	}, new(big.Int).SetBytes(hexutil.MustDecode(replacedGISTHash)))
	if err != nil {
		return nil, errors.Wrap(err, "failed to get replaced gist")
	}

	return m.StateUpdateMsgByStates(ctx, latestState, replacedState, latestGIST, replacedGIST)
}

func (m *StateUpdateMessageMaker) StateUpdateMsgByStates(_ context.Context, latestState, replacedState statebind.IStateStateInfo, latestGIST, replacedGIST statebind.IStateGistRootInfo) (*oracletypes.MsgCreateIdentityDefaultTransferOp, error) {
	if latestState.State.Cmp(replacedState.ReplacedByState) != 0 {
		return nil, errors.New("replaced state does not correspond latest state")
	}

	if latestGIST.Root.Cmp(replacedGIST.ReplacedByRoot) != 0 {
		return nil, errors.New("replaced gist does not correspond latest gist")
	}

	return &oracletypes.MsgCreateIdentityDefaultTransferOp{
		Creator:                 m.txCreatorAddr,
		Contract:                m.contract,
		Chain:                   m.homeChain,
		Id:                      hexutil.Encode(latestState.Id.Bytes()), // should be issuer id only
		GISTHash:                hexutil.Encode(latestGIST.Root.Bytes()),
		StateHash:               hexutil.Encode(latestState.State.Bytes()),
		StateCreatedAtTimestamp: latestState.CreatedAtTimestamp.String(),
		StateCreatedAtBlock:     latestState.CreatedAtBlock.String(),
		StateReplacedBy:         hexutil.Encode(latestState.ReplacedByState.Bytes()),
		GISTReplacedBy:          hexutil.Encode(latestGIST.ReplacedByRoot.Bytes()),
		GISTCreatedAtTimestamp:  latestGIST.CreatedAtTimestamp.String(),
		GISTCreatedAtBlock:      latestGIST.CreatedAtBlock.String(),
		ReplacedStateHash:       hexutil.Encode(replacedState.State.Bytes()),
		ReplacedGISTtHash:       hexutil.Encode(replacedGIST.Root.Bytes()),
	}, nil

}

func (m *StateUpdateMessageMaker) getStatesOnBlock(ctx context.Context, block *big.Int) (*statebind.IStateStateInfo, *statebind.IStateStateInfo, error) {
	length, err := m.stateDataProvider.GetStateInfoHistoryLengthById(&bind.CallOpts{
		Context: ctx,
	}, m.watchedIssuerID)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to get overall states count")
	}

	for {
		states, err := m.stateDataProvider.GetStateInfoHistoryById(&bind.CallOpts{
			Context: ctx,
		}, m.watchedIssuerID, new(big.Int).Sub(length, big.NewInt(2)), big.NewInt(2))

		if err != nil {
			return nil, nil, errors.Wrap(err, "failed to get last two states")
		}

		replacedState := states[0]
		latestState := states[1]

		if latestState.CreatedAtBlock.Cmp(block) == 0 {
			return &latestState, &replacedState, nil
		}

		// FIXME maybe increase step to reduce RPC calls amount
		length.Sub(length, big.NewInt(1))

		if length.Cmp(big.NewInt(1)) == 0 {
			return nil, nil, errors.Wrap(err, "requested state on block does noe exist")
		}
	}
}
