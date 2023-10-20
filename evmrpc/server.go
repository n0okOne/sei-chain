package evmrpc

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/sei-protocol/sei-chain/x/evm/keeper"
	"github.com/tendermint/tendermint/libs/log"
	rpcclient "github.com/tendermint/tendermint/rpc/client"
)

type EVMServer interface {
	Start() error
}

func NewEVMHTTPServer(
	logger log.Logger,
	addr string,
	port int,
	timeouts rpc.HTTPTimeouts,
	tmClient rpcclient.Client,
	k *keeper.Keeper,
	ctxProvider func(int64) sdk.Context,
	txDecoder sdk.TxDecoder,
) (EVMServer, error) {
	httpServer := newHTTPServer(logger, timeouts)
	if err := httpServer.setListenAddr(addr, port); err != nil {
		return nil, err
	}
	apis := []rpc.API{
		{
			Namespace: "echo",
			Service:   NewEchoAPI(),
		},
		{
			Namespace: "eth",
			Service:   NewBlockAPI(tmClient, k, ctxProvider, txDecoder),
		},
		{
			Namespace: "eth",
			Service:   NewTransactionAPI(tmClient, k, ctxProvider, txDecoder),
		},
		{
			Namespace: "eth",
			Service:   NewInfoAPI(tmClient, k, ctxProvider, txDecoder),
		},
		{
			Namespace: "eth",
			Service:   NewFilterAPI(tmClient, k, ctxProvider),
		},
	}
	if err := httpServer.enableRPC(apis, httpConfig{
		// TODO: add CORS configs and virtual host configs
	}); err != nil {
		return nil, err
	}
	return httpServer, nil
}

func NewEVMWebSocketServer(
	logger log.Logger,
	addr string,
	port int,
	origins []string,
	timeouts rpc.HTTPTimeouts,
) (EVMServer, error) {
	httpServer := newHTTPServer(logger, timeouts)
	if err := httpServer.setListenAddr(addr, port); err != nil {
		return nil, err
	}
	apis := []rpc.API{
		{
			Namespace: "echo",
			Service:   NewEchoAPI(),
		},
	}
	if err := httpServer.enableWS(apis, wsConfig{Origins: origins}); err != nil {
		return nil, err
	}
	return httpServer, nil
}
