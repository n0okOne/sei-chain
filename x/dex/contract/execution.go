package contract

import (
	"context"
	"fmt"
	"sync"
	"time"

	otrace "go.opentelemetry.io/otel/trace"

	"github.com/cosmos/cosmos-sdk/telemetry"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/sei-protocol/sei-chain/store/whitelist/multi"
	"github.com/sei-protocol/sei-chain/utils"
	"github.com/sei-protocol/sei-chain/utils/datastructures"
	dexcache "github.com/sei-protocol/sei-chain/x/dex/cache"
	"github.com/sei-protocol/sei-chain/x/dex/exchange"
	"github.com/sei-protocol/sei-chain/x/dex/keeper"
	dexkeeperabci "github.com/sei-protocol/sei-chain/x/dex/keeper/abci"
	dexkeeperutils "github.com/sei-protocol/sei-chain/x/dex/keeper/utils"
	"github.com/sei-protocol/sei-chain/x/dex/types"
	dextypesutils "github.com/sei-protocol/sei-chain/x/dex/types/utils"
	dextypeswasm "github.com/sei-protocol/sei-chain/x/dex/types/wasm"
	"go.opentelemetry.io/otel/attribute"
)

func CallPreExecutionHooks(
	ctx context.Context,
	sdkCtx sdk.Context,
	contractAddr string,
	dexkeeper *keeper.Keeper,
	registeredPairs []types.Pair,
	tracer *otrace.Tracer,
) error {
	spanCtx, span := (*tracer).Start(ctx, "PreExecutionHooks")
	defer span.End()
	span.SetAttributes(attribute.String("contract", contractAddr))
	abciWrapper := dexkeeperabci.KeeperWrapper{Keeper: dexkeeper}
	if err := abciWrapper.HandleEBCancelOrders(spanCtx, sdkCtx, tracer, contractAddr, registeredPairs); err != nil {
		return err
	}
	if err := abciWrapper.HandleEBPlaceOrders(spanCtx, sdkCtx, tracer, contractAddr, registeredPairs); err != nil {
		return err
	}
	return nil
}

func ExecutePair(
	ctx sdk.Context,
	contractAddr string,
	pair types.Pair,
	dexkeeper *keeper.Keeper,
	orderbook *types.OrderBook,
) []*types.SettlementEntry {
	typedContractAddr := dextypesutils.ContractAddress(contractAddr)
	typedPairStr := dextypesutils.GetPairString(&pair)

	cancelForPair(ctx, typedContractAddr, typedPairStr, dexkeeper, orderbook)
	marketOrderOutcome := matchMarketOrderForPair(ctx, typedContractAddr, typedPairStr, dexkeeper, orderbook)
	limitOrderOutcome := matchLimitOrderForPair(ctx, typedContractAddr, typedPairStr, dexkeeper, orderbook)
	totalOutcome := marketOrderOutcome.Merge(&limitOrderOutcome)

	dexkeeperutils.SetPriceStateFromExecutionOutcome(ctx, dexkeeper, typedContractAddr, pair, totalOutcome)
	dexkeeperutils.FlushOrderbook(ctx, dexkeeper, typedContractAddr, orderbook)

	return totalOutcome.Settlements
}

func cancelForPair(
	ctx sdk.Context,
	typedContractAddr dextypesutils.ContractAddress,
	typedPairStr dextypesutils.PairString,
	dexkeeper *keeper.Keeper,
	orderbook *types.OrderBook,
) {
	cancels := dexkeeper.MemState.GetBlockCancels(ctx, typedContractAddr, typedPairStr)
	exchange.CancelOrders(cancels.Get(), orderbook)
}

func matchMarketOrderForPair(
	ctx sdk.Context,
	typedContractAddr dextypesutils.ContractAddress,
	typedPairStr dextypesutils.PairString,
	dexkeeper *keeper.Keeper,
	orderbook *types.OrderBook,
) exchange.ExecutionOutcome {
	orders := dexkeeper.MemState.GetBlockOrders(ctx, typedContractAddr, typedPairStr)
	marketBuys := orders.GetSortedMarketOrders(types.PositionDirection_LONG, true)
	marketSells := orders.GetSortedMarketOrders(types.PositionDirection_SHORT, true)
	marketBuyOutcome := exchange.MatchMarketOrders(
		ctx,
		marketBuys,
		orderbook.Shorts,
		types.PositionDirection_LONG,
	)
	marketSellOutcome := exchange.MatchMarketOrders(
		ctx,
		marketSells,
		orderbook.Longs,
		types.PositionDirection_SHORT,
	)
	return marketBuyOutcome.Merge(&marketSellOutcome)
}

func matchLimitOrderForPair(
	ctx sdk.Context,
	typedContractAddr dextypesutils.ContractAddress,
	typedPairStr dextypesutils.PairString,
	dexkeeper *keeper.Keeper,
	orderbook *types.OrderBook,
) exchange.ExecutionOutcome {
	orders := dexkeeper.MemState.GetBlockOrders(ctx, typedContractAddr, typedPairStr)
	limitBuys := orders.GetLimitOrders(types.PositionDirection_LONG)
	limitSells := orders.GetLimitOrders(types.PositionDirection_SHORT)
	return exchange.MatchLimitOrders(
		ctx,
		limitBuys,
		limitSells,
		orderbook,
	)
}

func GetMatchResults(
	ctx sdk.Context,
	typedContractAddr dextypesutils.ContractAddress,
	typedPairStr dextypesutils.PairString,
	dexkeeper *keeper.Keeper,
) ([]*types.Order, []*types.Cancellation) {
	orderResults := []*types.Order{}
	orders := dexkeeper.MemState.GetBlockOrders(ctx, typedContractAddr, typedPairStr)
	// First add any new order, whether successfully placed or not, to the store
	for _, order := range orders.Get() {
		if order.Quantity.IsZero() {
			order.Status = types.OrderStatus_FULFILLED
		}
		orderResults = append(orderResults, order)
	}
	// Then update order status and insert cancel record for any cancellation
	cancelResults := dexkeeper.MemState.GetBlockCancels(ctx, typedContractAddr, typedPairStr).Get()
	return orderResults, cancelResults
}

func GetOrderIDToSettledQuantities(settlements []*types.SettlementEntry) map[uint64]sdk.Dec {
	res := map[uint64]sdk.Dec{}
	for _, settlement := range settlements {
		if _, ok := res[settlement.OrderId]; !ok {
			res[settlement.OrderId] = sdk.ZeroDec()
		}
		res[settlement.OrderId] = res[settlement.OrderId].Add(settlement.Quantity)
	}
	return res
}

func ExecutePairsInParallel(ctx sdk.Context, contractAddr string, dexkeeper *keeper.Keeper, registeredPairs []types.Pair, orderBooks *datastructures.TypedSyncMap[dextypesutils.PairString, *types.OrderBook]) ([]*types.SettlementEntry, []*types.Cancellation) {
	typedContractAddr := dextypesutils.ContractAddress(contractAddr)
	orderResults := []*types.Order{}
	cancelResults := []*types.Cancellation{}
	settlements := []*types.SettlementEntry{}

	mu := sync.Mutex{}
	wg := sync.WaitGroup{}
	anyPanicked := false

	for _, pair := range registeredPairs {
		wg.Add(1)

		pair := pair
		pairCtx := ctx.WithMultiStore(multi.NewStore(ctx.MultiStore(), GetPerPairWhitelistMap(contractAddr, pair))).WithEventManager(sdk.NewEventManager())
		go func() {
			defer wg.Done()
			defer utils.PanicHandler(func(err any) {
				mu.Lock()
				defer mu.Unlock()
				anyPanicked = true
				utils.MetricsPanicCallback(err, ctx, fmt.Sprintf("%s-%s|%s", contractAddr, pair.PriceDenom, pair.AssetDenom))
			})()

			pairCopy := pair
			pairStr := dextypesutils.GetPairString(&pairCopy)
			orderbook, found := orderBooks.Load(pairStr)
			if !found {
				panic(fmt.Sprintf("Orderbook not found for %s", pairStr))
			}
			pairSettlements := ExecutePair(pairCtx, contractAddr, pair, dexkeeper, orderbook.DeepCopy())
			orderIDToSettledQuantities := GetOrderIDToSettledQuantities(pairSettlements)
			PrepareCancelUnfulfilledMarketOrders(pairCtx, typedContractAddr, pairStr, dexkeeper, orderIDToSettledQuantities)

			mu.Lock()
			defer mu.Unlock()
			orders, cancels := GetMatchResults(ctx, typedContractAddr, dextypesutils.GetPairString(&pairCopy), dexkeeper)
			orderResults = append(orderResults, orders...)
			cancelResults = append(cancelResults, cancels...)
			settlements = append(settlements, pairSettlements...)
			// ordering of events doesn't matter since events aren't part of consensus
			ctx.EventManager().EmitEvents(pairCtx.EventManager().Events())
		}()
	}
	wg.Wait()
	if anyPanicked {
		// need to re-throw panic to the top level goroutine
		panic("panicked during pair execution")
	}
	dexkeeper.SetMatchResult(ctx, contractAddr, types.NewMatchResult(orderResults, cancelResults, settlements))

	return settlements, cancelResults
}

func HandleExecutionForContract(
	ctx context.Context,
	sdkCtx sdk.Context,
	contract types.ContractInfo,
	dexkeeper *keeper.Keeper,
	registeredPairs []types.Pair,
	orderBooks *datastructures.TypedSyncMap[dextypesutils.PairString, *types.OrderBook],
	tracer *otrace.Tracer,
) (map[string]dextypeswasm.ContractOrderResult, []*types.SettlementEntry, error) {
	executionStart := time.Now()
	defer telemetry.ModuleSetGauge(types.ModuleName, float32(time.Since(executionStart).Milliseconds()), "handle_execution_for_contract_ms")
	contractAddr := contract.ContractAddr
	typedContractAddr := dextypesutils.ContractAddress(contractAddr)
	orderResults := map[string]dextypeswasm.ContractOrderResult{}

	// Call contract hooks so that contracts can do internal bookkeeping
	if err := CallPreExecutionHooks(ctx, sdkCtx, contractAddr, dexkeeper, registeredPairs, tracer); err != nil {
		return orderResults, []*types.SettlementEntry{}, err
	}

	settlements, cancellations := ExecutePairsInParallel(sdkCtx, contractAddr, dexkeeper, registeredPairs, orderBooks)
	defer EmitSettlementMetrics(settlements)

	// populate order placement results for FinalizeBlock hook
	dexkeeper.MemState.GetAllBlockOrders(sdkCtx, typedContractAddr).DeepApply(func(orders *dexcache.BlockOrders) {
		dextypeswasm.PopulateOrderPlacementResults(contractAddr, orders.Get(), cancellations, orderResults)
	})
	dextypeswasm.PopulateOrderExecutionResults(contractAddr, settlements, orderResults)

	return orderResults, settlements, nil
}

// Emit metrics for settlements
func EmitSettlementMetrics(settlements []*types.SettlementEntry) {
	if len(settlements) > 0 {
		telemetry.ModuleSetGauge(
			types.ModuleName,
			float32(len(settlements)),
			"num_settlements",
		)
		var totalQuantity int
		for _, s := range settlements {
			totalQuantity += s.Quantity.Size()
			telemetry.IncrCounter(
				1,
				"num_settlements_order_type_"+s.GetOrderType(),
			)
			telemetry.IncrCounter(
				1,
				"num_settlements_position_direction"+s.GetPositionDirection(),
			)
			telemetry.IncrCounter(
				1,
				"num_settlements_asset_denom_"+s.GetAssetDenom(),
			)
			telemetry.IncrCounter(
				1,
				"num_settlements_price_denom_"+s.GetPriceDenom(),
			)
		}
		telemetry.ModuleSetGauge(
			types.ModuleName,
			float32(totalQuantity),
			"num_total_order_quantity_in_settlements",
		)
	}
}
