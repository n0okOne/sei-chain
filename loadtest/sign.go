package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/cosmos/cosmos-sdk/client"
	clienttx "github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	xauthsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
)

const NodeURI = "tcp://localhost:26657"

type AccountInfo struct {
	Address  string `json:"address"`
	Mnemonic string `json:"mnemonic"`
}

type SignerInfo struct {
	AccountNumber  uint64
	SequenceNumber uint64
	mutex          *sync.Mutex
}

func NewSignerInfo(accountNumber uint64, sequenceNumber uint64) *SignerInfo {
	return &SignerInfo{
		AccountNumber:  accountNumber,
		SequenceNumber: sequenceNumber,
		mutex:          &sync.Mutex{},
	}
}

func (si *SignerInfo) IncrementAccountNumber() {
	si.mutex.Lock()
	defer si.mutex.Unlock()
	si.AccountNumber++
}

type SignerClient struct {
	CachedAccountSeqNum *sync.Map
	CachedAccountKey    *sync.Map
}

func NewSignerClient() *SignerClient {
	return &SignerClient{
		CachedAccountSeqNum: &sync.Map{},
		CachedAccountKey:    &sync.Map{},
	}
}

type Validator struct {
	OpperatorAddr string `json:"operator_address"`
}

type QueryValidators struct {
	Validators []Validator `json:"validators"`
}

func GetValidators() QueryValidators {
	seidQuery, err := exec.Command("seid", "query", "staking", "validators", "--output", "json").Output()
	if err != nil {
		panic(err)
	}

	qv := QueryValidators{}
	if err := json.Unmarshal(seidQuery, &qv); err != nil {
		panic(err)
	}

	return qv
}

func (sc *SignerClient) GetKey(accountIdx uint64) cryptotypes.PrivKey {
	if val, ok := sc.CachedAccountKey.Load(accountIdx); ok {
		privKey := val.(cryptotypes.PrivKey)
		return privKey
	}
	userHomeDir, _ := os.UserHomeDir()
	accountKeyFilePath := filepath.Join(userHomeDir, "test_accounts", fmt.Sprintf("ta%d.json", accountIdx))
	jsonFile, err := os.Open(accountKeyFilePath)
	if err != nil {
		panic(err)
	}
	var accountInfo AccountInfo
	byteVal, err := io.ReadAll(jsonFile)
	if err != nil {
		panic(err)
	}
	jsonFile.Close()
	if err := json.Unmarshal(byteVal, &accountInfo); err != nil {
		panic(err)
	}
	kr, _ := keyring.New(sdk.KeyringServiceName(), "test", filepath.Join(userHomeDir, ".sei"), os.Stdin)
	keyringAlgos, _ := kr.SupportedAlgorithms()
	algoStr := string(hd.Sr25519Type)
	algo, _ := keyring.NewSigningAlgoFromString(algoStr, keyringAlgos)
	hdpath := hd.CreateHDPath(sdk.GetConfig().GetCoinType(), 0, 0).String()
	derivedPriv, _ := algo.Derive()(accountInfo.Mnemonic, "", hdpath)
	privKey := algo.Generate()(derivedPriv)

	// Cache this so we don't need to regenerate it
	sc.CachedAccountKey.Store(accountIdx, privKey)
	return privKey
}

func (sc *SignerClient) SignTx(chainID string, txBuilder *client.TxBuilder, privKey cryptotypes.PrivKey, seqDelta uint64) {
	var sigsV2 []signing.SignatureV2
	signerInfo := sc.GetAccountNumberSequenceNumber(privKey)
	accountNum := signerInfo.AccountNumber
	seqNum := signerInfo.SequenceNumber

	seqNum += seqDelta
	sigV2 := signing.SignatureV2{
		PubKey: privKey.PubKey(),
		Data: &signing.SingleSignatureData{
			SignMode:  TestConfig.TxConfig.SignModeHandler().DefaultMode(),
			Signature: nil,
		},
		Sequence: seqNum,
	}
	sigsV2 = append(sigsV2, sigV2)
	_ = (*txBuilder).SetSignatures(sigsV2...)
	sigsV2 = []signing.SignatureV2{}
	signerData := xauthsigning.SignerData{
		ChainID:       chainID,
		AccountNumber: accountNum,
		Sequence:      seqNum,
	}
	sigV2, _ = clienttx.SignWithPrivKey(
		TestConfig.TxConfig.SignModeHandler().DefaultMode(),
		signerData,
		*txBuilder,
		privKey,
		TestConfig.TxConfig,
		seqNum,
	)
	sigsV2 = append(sigsV2, sigV2)
	_ = (*txBuilder).SetSignatures(sigsV2...)
}

func (sc *SignerClient) GetAccountNumberSequenceNumber(privKey cryptotypes.PrivKey) SignerInfo {
	if val, ok := sc.CachedAccountSeqNum.Load(privKey); ok {
		signerinfo := val.(SignerInfo)
		signerinfo.IncrementAccountNumber()
		return signerinfo
	}

	hexAccount := privKey.PubKey().Address()
	address, err := sdk.AccAddressFromHex(hexAccount.String())
	if err != nil {
		panic(err)
	}
	accountRetriever := authtypes.AccountRetriever{}
	cl, err := client.NewClientFromNode(NodeURI)
	if err != nil {
		panic(err)
	}
	context := client.Context{}
	context = context.WithNodeURI(NodeURI)
	context = context.WithClient(cl)
	context = context.WithInterfaceRegistry(TestConfig.InterfaceRegistry)
	userHomeDir, _ := os.UserHomeDir()
	kr, _ := keyring.New(sdk.KeyringServiceName(), "test", filepath.Join(userHomeDir, ".sei"), os.Stdin)
	context = context.WithKeyring(kr)
	account, seq, err := accountRetriever.GetAccountNumberSequence(context, address)
	if err != nil {
		time.Sleep(5 * time.Second)
		// retry once after 5 seconds
		account, seq, err = accountRetriever.GetAccountNumberSequence(context, address)
		if err != nil {
			panic(err)
		}
	}

	signerInfo := *NewSignerInfo(account, seq)
	sc.CachedAccountSeqNum.Store(privKey, signerInfo)
	return signerInfo
}
