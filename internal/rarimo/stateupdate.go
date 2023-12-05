package rarimo

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common/hexutil"

	statebind "github.com/rarimo/evm-identity-saver-svc/pkg/state"
	oracletypes "github.com/rarimo/rarimo-core/x/oraclemanager/types"
	"gitlab.com/distributed_lab/logan/v3/errors"
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
	statesPerRequest  int64
	stateDataProvider StateDataProvider
}

func NewStateUpdateMessageMaker(txCreatorAddr string, contract string, homeChain string, statesPerRequest int64, stateDataProvider StateDataProvider) *StateUpdateMessageMaker {
	return &StateUpdateMessageMaker{txCreatorAddr: txCreatorAddr, contract: contract, homeChain: homeChain, statesPerRequest: statesPerRequest, stateDataProvider: stateDataProvider}
}

func (m *StateUpdateMessageMaker) StateUpdateMsgByBlock(ctx context.Context, issuer, block *big.Int) (*oracletypes.MsgCreateIdentityStateTransferOp, error) {
	latestState, replacedState, err := m.getStatesOnBlock(ctx, issuer, block)
	if err != nil {
		return nil, err
	}

	if latestState == nil || replacedState == nil {
		return nil, nil
	}

	return m.StateUpdateMsgByStates(ctx, *latestState, *replacedState)
}

func (m *StateUpdateMessageMaker) GISTUpdateMsgByBlock(ctx context.Context, block *big.Int) (*oracletypes.MsgCreateIdentityGISTTransferOp, error) {
	latestGIST, replacedGIST, err := m.getGISTsOnBlock(ctx, block)
	if err != nil {
		return nil, err
	}

	if latestGIST == nil || replacedGIST == nil {
		return nil, nil
	}

	return m.GISTUpdateMsgByGISTs(ctx, *latestGIST, *replacedGIST)
}

func (m *StateUpdateMessageMaker) StateUpdateMsgByHashes(ctx context.Context, issuer, latestStateHash, replacedStateHash string) (*oracletypes.MsgCreateIdentityStateTransferOp, error) {
	latestState, err := m.stateDataProvider.GetStateInfoByIdAndState(&bind.CallOpts{
		Context: ctx,
	}, new(big.Int).SetBytes(hexutil.MustDecode(issuer)), new(big.Int).SetBytes(hexutil.MustDecode(latestStateHash)))
	if err != nil {
		return nil, errors.Wrap(err, "failed to get latest state")
	}

	replacedState, err := m.stateDataProvider.GetStateInfoByIdAndState(&bind.CallOpts{
		Context: ctx,
	}, new(big.Int).SetBytes(hexutil.MustDecode(issuer)), new(big.Int).SetBytes(hexutil.MustDecode(replacedStateHash)))
	if err != nil {
		return nil, errors.Wrap(err, "failed to get replaced state")
	}

	return m.StateUpdateMsgByStates(ctx, latestState, replacedState)
}

func (m *StateUpdateMessageMaker) GISTUpdateMsgByHashes(ctx context.Context, latestGISTHash, replacedGISTHash string) (*oracletypes.MsgCreateIdentityGISTTransferOp, error) {
	latestGIST, err := m.stateDataProvider.GetGISTRootInfo(&bind.CallOpts{
		Context: ctx,
	}, new(big.Int).SetBytes(hexutil.MustDecode(latestGISTHash)))
	if err != nil {
		return nil, errors.Wrap(err, "failed to get latest state")
	}

	replacedGIST, err := m.stateDataProvider.GetGISTRootInfo(&bind.CallOpts{
		Context: ctx,
	}, new(big.Int).SetBytes(hexutil.MustDecode(replacedGISTHash)))
	if err != nil {
		return nil, errors.Wrap(err, "failed to get replaced state")
	}

	return m.GISTUpdateMsgByGISTs(ctx, latestGIST, replacedGIST)
}

func (m *StateUpdateMessageMaker) StateUpdateMsgByStates(_ context.Context, latestState, replacedState statebind.IStateStateInfo) (*oracletypes.MsgCreateIdentityStateTransferOp, error) {
	if latestState.State.Cmp(replacedState.ReplacedByState) != 0 {
		return nil, errors.New("replaced state does not correspond latest state")
	}

	return &oracletypes.MsgCreateIdentityStateTransferOp{
		Creator:                 m.txCreatorAddr,
		Contract:                m.contract,
		Chain:                   m.homeChain,
		Id:                      hexutil.Encode(latestState.Id.Bytes()), // should be issuer id only
		StateHash:               hexutil.Encode(latestState.State.Bytes()),
		StateCreatedAtTimestamp: latestState.CreatedAtTimestamp.String(),
		StateCreatedAtBlock:     latestState.CreatedAtBlock.String(),
		ReplacedStateHash:       hexutil.Encode(replacedState.State.Bytes()),
	}, nil
}

func (m *StateUpdateMessageMaker) GISTUpdateMsgByGISTs(_ context.Context, latestGIST, replacedGIST statebind.IStateGistRootInfo) (*oracletypes.MsgCreateIdentityGISTTransferOp, error) {
	if latestGIST.Root.Cmp(replacedGIST.ReplacedByRoot) != 0 {
		return nil, errors.New("replaced gist does not correspond latest state")
	}

	return &oracletypes.MsgCreateIdentityGISTTransferOp{
		Creator:                m.txCreatorAddr,
		Contract:               m.contract,
		Chain:                  m.homeChain,
		GISTHash:               hexutil.Encode(latestGIST.Root.Bytes()),
		GISTCreatedAtTimestamp: latestGIST.CreatedAtTimestamp.String(),
		GISTCreatedAtBlock:     latestGIST.CreatedAtBlock.String(),
		ReplacedGISTtHash:      hexutil.Encode(replacedGIST.Root.Bytes()),
	}, nil
}

func (m *StateUpdateMessageMaker) getStatesOnBlock(ctx context.Context, issuer, block *big.Int) (*statebind.IStateStateInfo, *statebind.IStateStateInfo, error) {
	length, err := m.stateDataProvider.GetStateInfoHistoryLengthById(&bind.CallOpts{
		Context: ctx,
	}, issuer)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to get overall states count")
	}

	if length.Cmp(big.NewInt(m.statesPerRequest)) == 0 {
		// We need more states on contract. Ignore that state transition.
		return nil, nil, nil
	}

	length = new(big.Int).Sub(length, big.NewInt(m.statesPerRequest))

	for {
		states, err := m.stateDataProvider.GetStateInfoHistoryById(&bind.CallOpts{
			Context: ctx,
		}, issuer, length, big.NewInt(m.statesPerRequest))

		if err != nil {
			return nil, nil, errors.Wrap(err, "failed to get last two states")
		}

		for i := 1; i < len(states); i++ {
			replacedState := states[0]
			latestState := states[1]
			if latestState.CreatedAtBlock.Cmp(block) == 0 {
				return &latestState, &replacedState, nil
			}
		}

		length = new(big.Int).Sub(length, big.NewInt(m.statesPerRequest-1))
		if length.Cmp(big.NewInt(1)) <= 0 {
			return nil, nil, errors.Wrap(err, "requested state on block does not exist")
		}
	}
}

func (m *StateUpdateMessageMaker) getGISTsOnBlock(ctx context.Context, block *big.Int) (*statebind.IStateGistRootInfo, *statebind.IStateGistRootInfo, error) {
	length, err := m.stateDataProvider.GetGISTRootHistoryLength(&bind.CallOpts{
		Context: ctx,
	})
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to get overall gists count")
	}

	if length.Cmp(big.NewInt(m.statesPerRequest)) == 0 {
		// We need more states on contract. Ignore that state transition.
		return nil, nil, nil
	}

	length = new(big.Int).Sub(length, big.NewInt(m.statesPerRequest))

	for {
		gists, err := m.stateDataProvider.GetGISTRootHistory(&bind.CallOpts{
			Context: ctx,
		}, length, big.NewInt(m.statesPerRequest))

		if err != nil {
			return nil, nil, errors.Wrap(err, "failed to get last two gists")
		}

		for i := 1; i < len(gists); i++ {
			replacedGIST := gists[i-1]
			latestGIST := gists[i]

			if latestGIST.CreatedAtBlock.Cmp(block) == 0 {
				return &latestGIST, &replacedGIST, nil
			}
		}

		length = new(big.Int).Sub(length, big.NewInt(m.statesPerRequest-1))
		if length.Cmp(big.NewInt(1)) <= 0 {
			return nil, nil, errors.Wrap(err, "requested gist on block does not exist")
		}
	}
}
