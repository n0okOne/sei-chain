package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
)

const (
	// module name
	ModuleName = "evm"

	RouterKey = ModuleName

	// StoreKey is string representation of the store key for auth
	StoreKey = "evm"

	// QuerierRoute is the querier route for auth
	QuerierRoute = ModuleName
)

var (
	EVMAddressToSeiAddressKeyPrefix = []byte{0x01}
	SeiAddressToEVMAddressKeyPrefix = []byte{0x02}
	StateKeyPrefix                  = []byte{0x03}
	TransientStateKeyPrefix         = []byte{0x04}
	AccountTransientStateKeyPrefix  = []byte{0x05}
	TransientModuleStateKeyPrefix   = []byte{0x06}
	CodeKeyPrefix                   = []byte{0x07}
	CodeHashKeyPrefix               = []byte{0x08}
	CodeSizeKeyPrefix               = []byte{0x09}
	NonceKeyPrefix                  = []byte{0x0a}
	ReceiptKeyPrefix                = []byte{0x0b}
)

func EVMAddressToSeiAddressKey(evmAddress common.Address) []byte {
	return append(EVMAddressToSeiAddressKeyPrefix, evmAddress[:]...)
}

func SeiAddressToEVMAddressKey(seiAddress sdk.AccAddress) []byte {
	return append(SeiAddressToEVMAddressKeyPrefix, seiAddress...)
}

func StateKey(evmAddress common.Address) []byte {
	return append(StateKeyPrefix, evmAddress[:]...)
}

func TransientStateKey(evmAddress common.Address) []byte {
	return append(TransientStateKeyPrefix, evmAddress[:]...)
}

func ReceiptKey(txHash common.Hash) []byte {
	return append(ReceiptKeyPrefix, txHash[:]...)
}