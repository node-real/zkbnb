package executor

import (
	"bytes"
	"encoding/json"
	"errors"
	"math/big"

	"github.com/zeromicro/go-zero/core/logx"

	"github.com/bnb-chain/zkbnb-crypto/ffmath"
	"github.com/bnb-chain/zkbnb-crypto/wasm/txtypes"
	common2 "github.com/bnb-chain/zkbnb/common"
	"github.com/bnb-chain/zkbnb/dao/tx"
	"github.com/bnb-chain/zkbnb/types"
)

type DepositExecutor struct {
	BaseExecutor

	TxInfo *txtypes.DepositTxInfo
}

func NewDepositExecutor(bc IBlockchain, tx *tx.Tx) (TxExecutor, error) {
	txInfo, err := types.ParseDepositTxInfo(tx.TxInfo)
	if err != nil {
		logx.Errorf("parse deposit tx failed: %s", err.Error())
		return nil, errors.New("invalid tx info")
	}

	return &DepositExecutor{
		BaseExecutor: NewBaseExecutor(bc, tx, txInfo),
		TxInfo:       txInfo,
	}, nil
}
func (e *DepositExecutor) SetTxInfo(info *txtypes.DepositTxInfo) {
	e.TxInfo = info
}

func (e *DepositExecutor) Prepare() error {
	bc := e.bc
	txInfo := e.TxInfo

	// The account index from txInfo isn't true, find account by l1Address.
	l1Address := txInfo.L1Address
	account, err := bc.StateDB().GetAccountByL1Address(l1Address)
	if err != nil {
		return err
	}

	// Set the right account index.
	txInfo.AccountIndex = account.AccountIndex

	// Mark the tree states that would be affected in this executor.
	e.MarkAccountAssetsDirty(txInfo.AccountIndex, []int64{txInfo.AssetId})
	return e.BaseExecutor.Prepare()
}

func (e *DepositExecutor) VerifyInputs(skipGasAmtChk, skipSigChk bool) error {
	txInfo := e.TxInfo

	if txInfo.AssetAmount.Cmp(types.ZeroBigInt) < 0 {
		return types.AppErrInvalidAssetAmount
	}

	return nil
}

func (e *DepositExecutor) ApplyTransaction() error {
	bc := e.bc
	txInfo := e.TxInfo

	depositAccount, err := bc.StateDB().GetFormatAccount(txInfo.AccountIndex)
	if err != nil {
		return err
	}
	depositAccount.AssetInfo[txInfo.AssetId].Balance = ffmath.Add(depositAccount.AssetInfo[txInfo.AssetId].Balance, txInfo.AssetAmount)

	stateCache := e.bc.StateDB()
	stateCache.SetPendingAccount(depositAccount.AccountIndex, depositAccount)
	return e.BaseExecutor.ApplyTransaction()
}

func (e *DepositExecutor) GeneratePubData() error {
	txInfo := e.TxInfo

	var buf bytes.Buffer
	buf.WriteByte(uint8(types.TxTypeDeposit))
	buf.Write(common2.Uint32ToBytes(uint32(txInfo.AccountIndex)))
	buf.Write(common2.AddressStrToBytes(txInfo.L1Address))
	buf.Write(common2.Uint16ToBytes(uint16(txInfo.AssetId)))
	buf.Write(common2.Uint128ToBytes(txInfo.AssetAmount))

	pubData := common2.SuffixPaddingBuToPubdataSize(buf.Bytes())

	stateCache := e.bc.StateDB()
	stateCache.PriorityOperations++
	stateCache.PubDataOffset = append(stateCache.PubDataOffset, uint32(len(stateCache.PubData)))
	stateCache.PubData = append(stateCache.PubData, pubData...)
	return nil
}

func (e *DepositExecutor) GetExecutedTx(fromApi bool) (*tx.Tx, error) {
	txInfoBytes, err := json.Marshal(e.TxInfo)
	if err != nil {
		logx.Errorf("unable to marshal tx, err: %s", err.Error())
		return nil, errors.New("unmarshal tx failed")
	}

	e.tx.TxInfo = string(txInfoBytes)
	e.tx.AssetId = e.TxInfo.AssetId
	e.tx.TxAmount = e.TxInfo.AssetAmount.String()
	e.tx.AccountIndex = e.TxInfo.AccountIndex
	return e.BaseExecutor.GetExecutedTx(fromApi)
}

func (e *DepositExecutor) GenerateTxDetails() ([]*tx.TxDetail, error) {
	txInfo := e.TxInfo
	depositAccount, err := e.bc.StateDB().GetFormatAccount(txInfo.AccountIndex)
	if err != nil {
		return nil, err
	}
	baseBalance := depositAccount.AssetInfo[txInfo.AssetId]
	deltaBalance := &types.AccountAsset{
		AssetId:                  txInfo.AssetId,
		Balance:                  txInfo.AssetAmount,
		OfferCanceledOrFinalized: big.NewInt(0),
	}
	txDetail := &tx.TxDetail{
		AssetId:         txInfo.AssetId,
		AssetType:       types.FungibleAssetType,
		AccountIndex:    txInfo.AccountIndex,
		L1Address:       depositAccount.L1Address,
		Balance:         baseBalance.String(),
		BalanceDelta:    deltaBalance.String(),
		Order:           0,
		AccountOrder:    0,
		Nonce:           depositAccount.Nonce,
		CollectionNonce: depositAccount.CollectionNonce,
	}
	return []*tx.TxDetail{txDetail}, nil
}

func (e *ChangePubKeyExecutor) Finalize() error {
	bc := e.bc
	txInfo := e.TxInfo
	bc.StateDB().AccountAssetTrees.UpdateCache(txInfo.AccountIndex, bc.CurrentBlock().BlockHeight)
	return nil
}
