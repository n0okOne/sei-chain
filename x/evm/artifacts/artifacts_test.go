package artifacts_test

import (
	"encoding/hex"
	"math/big"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/sei-protocol/sei-chain/app"
	testkeeper "github.com/sei-protocol/sei-chain/testutil/keeper"
	"github.com/sei-protocol/sei-chain/x/evm/ante"
	"github.com/sei-protocol/sei-chain/x/evm/artifacts"
	"github.com/sei-protocol/sei-chain/x/evm/keeper"
	"github.com/sei-protocol/sei-chain/x/evm/types"
	"github.com/sei-protocol/sei-chain/x/evm/types/ethtx"
	"github.com/stretchr/testify/require"
	tmtypes "github.com/tendermint/tendermint/proto/tendermint/types"
)

// send 1 token to 0x34b575c2eaae50b81375f077517e6490adbd9735
const CallData = "0xa9059cbb00000000000000000000000034b575c2eaae50b81375f077517e6490adbd97350000000000000000000000000000000000000000000000000000000000000001"

func TestSimple(t *testing.T) {
	bytecode := artifacts.GetNativeSeiTokensERC20Bin()
	abi, err := artifacts.ArtifactsMetaData.GetAbi()
	args, err := abi.Pack("", "test")
	contractData := append(bytecode, args...)

	testApp := app.Setup(false, true)
	ctx := testApp.NewContext(false, tmtypes.Header{}).WithBlockHeight(2)
	k := &testApp.EvmKeeper
	privKey := testkeeper.MockPrivateKey()
	testPrivHex := hex.EncodeToString(privKey.Bytes())
	key, _ := crypto.HexToECDSA(testPrivHex)
	txData := ethtypes.LegacyTx{
		GasPrice: big.NewInt(1000000000000),
		Gas:      20000000,
		To:       nil,
		Value:    big.NewInt(0),
		Data:     contractData,
		Nonce:    0,
	}
	chainID := k.ChainID(ctx)
	evmParams := k.GetParams(ctx)
	chainCfg := evmParams.GetChainConfig()
	ethCfg := chainCfg.EthereumConfig(chainID)
	blockNum := big.NewInt(ctx.BlockHeight())
	signer := ethtypes.MakeSigner(ethCfg, blockNum, uint64(ctx.BlockTime().Unix()))
	tx, err := ethtypes.SignTx(ethtypes.NewTx(&txData), signer, key)
	require.Nil(t, err)
	txwrapper, err := ethtx.NewLegacyTx(tx)
	require.Nil(t, err)
	req, err := types.NewMsgEVMTransaction(txwrapper)
	require.Nil(t, err)

	_, evmAddr := testkeeper.PrivateKeyToAddresses(privKey)
	amt := sdk.NewCoins(sdk.NewCoin(k.GetBaseDenom(ctx), sdk.NewInt(200000000)))
	require.Nil(t, k.BankKeeper().MintCoins(ctx, types.ModuleName, sdk.NewCoins(sdk.NewCoin(k.GetBaseDenom(ctx), sdk.NewInt(200000000)))))
	require.Nil(t, k.BankKeeper().SendCoinsFromModuleToAccount(ctx, types.ModuleName, evmAddr[:], amt))
	require.Nil(t, k.BankKeeper().MintCoins(ctx, types.ModuleName, sdk.NewCoins(sdk.NewCoin("test", sdk.NewInt(200000000)))))
	require.Nil(t, k.BankKeeper().SendCoinsFromModuleToAccount(ctx, types.ModuleName, evmAddr[:], sdk.NewCoins(sdk.NewCoin("test", sdk.NewInt(200000000)))))

	msgServer := keeper.NewMsgServerImpl(k)

	// Deploy Simple Storage contract
	ante.Preprocess(ctx, req, k.GetParams(ctx), k.DecrementPendingTxCount)
	res, err := msgServer.EVMTransaction(sdk.WrapSDKContext(ctx), req)
	require.Nil(t, err)
	require.Empty(t, res.VmError)

	receipt, err := k.GetReceipt(ctx, common.HexToHash(res.Hash))
	require.Nil(t, err)
	require.NotNil(t, receipt)
	require.Equal(t, uint32(ethtypes.ReceiptStatusSuccessful), receipt.Status)

	// send transaction to the contract
	contractAddr := common.HexToAddress(receipt.ContractAddress)
	data, err := hex.DecodeString(CallData[2:])
	require.Nil(t, err)
	txData = ethtypes.LegacyTx{
		GasPrice: big.NewInt(1000000000000),
		Gas:      2000000,
		To:       &contractAddr,
		Value:    big.NewInt(0),
		Data:     data,
		Nonce:    1,
	}
	tx, err = ethtypes.SignTx(ethtypes.NewTx(&txData), signer, key)
	require.Nil(t, err)
	txwrapper, err = ethtx.NewLegacyTx(tx)
	require.Nil(t, err)
	req, err = types.NewMsgEVMTransaction(txwrapper)
	require.Nil(t, err)
	ante.Preprocess(ctx, req, k.GetParams(ctx), k.DecrementPendingTxCount)
	res, err = msgServer.EVMTransaction(sdk.WrapSDKContext(ctx), req)
	require.Nil(t, err)
	require.Empty(t, res.VmError)
}
