package utils

import (
	"sort"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/sei-protocol/sei-chain/x/dex/keeper"
	"github.com/sei-protocol/sei-chain/x/dex/types"
	dextypesutils "github.com/sei-protocol/sei-chain/x/dex/types/utils"
)

func PopulateOrderbook(
	ctx sdk.Context,
	keeper *keeper.Keeper,
	contractAddr dextypesutils.ContractAddress,
	pair types.Pair,
) *types.OrderBook {
	longs := keeper.GetAllLongBookForPair(ctx, string(contractAddr), pair.PriceDenom, pair.AssetDenom)
	shorts := keeper.GetAllShortBookForPair(ctx, string(contractAddr), pair.PriceDenom, pair.AssetDenom)
	sortOrderBookEntries(longs)
	sortOrderBookEntries(shorts)
	return &types.OrderBook{
		Longs: &types.CachedSortedOrderBookEntries{
			Entries:      longs,
			DirtyEntries: map[string]types.OrderBookEntry{},
		},
		Shorts: &types.CachedSortedOrderBookEntries{
			Entries:      shorts,
			DirtyEntries: map[string]types.OrderBookEntry{},
		},
	}
}

func sortOrderBookEntries(entries []types.OrderBookEntry) {
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].GetPrice().LT(entries[j].GetPrice())
	})
}

func FlushOrderbook(
	ctx sdk.Context,
	keeper *keeper.Keeper,
	typedContractAddr dextypesutils.ContractAddress,
	orderbook *types.OrderBook,
) {
	contractAddr := string(typedContractAddr)
	for _, entry := range orderbook.Longs.DirtyEntries {
		if entry.GetEntry().Quantity.IsZero() {
			keeper.RemoveLongBookByPrice(ctx, contractAddr, entry.GetEntry().Price, entry.GetEntry().PriceDenom, entry.GetEntry().AssetDenom)
		} else {
			longOrder := entry.(*types.LongBook)
			keeper.SetLongBook(ctx, contractAddr, *longOrder)
		}
	}
	for _, entry := range orderbook.Shorts.DirtyEntries {
		if entry.GetEntry().Quantity.IsZero() {
			keeper.RemoveShortBookByPrice(ctx, contractAddr, entry.GetEntry().Price, entry.GetEntry().PriceDenom, entry.GetEntry().AssetDenom)
		} else {
			shortOrder := entry.(*types.ShortBook)
			keeper.SetShortBook(ctx, contractAddr, *shortOrder)
		}
	}
}
