package esi

import (
	"context"
	"fmt"
	"strconv"
	"sync"
)

// RegionOrder is one market order from a region's public order book.
type RegionOrder struct {
	OrderId      int64   `json:"order_id"`
	TypeId       int64   `json:"type_id"`
	LocationId   int64   `json:"location_id"`
	SystemId     int64   `json:"system_id"`
	IsBuyOrder   bool    `json:"is_buy_order"`
	Price        float64 `json:"price"`
	VolumeRemain int64   `json:"volume_remain"`
}

// regionOrderWorkers is how many pages of a region's orders are fetched at
// once. The Forge has a few hundred pages, which takes minutes one at a time.
const regionOrderWorkers = 8

// GetRegionSellOrders returns every sell order in the region. Orders move
// between pages while they're fetched, so an order may show up twice; the
// copies are dropped.
func (c *Client) GetRegionSellOrders(ctx context.Context, regionId int64) ([]RegionOrder, error) {
	baseUrl := fmt.Sprintf("%s/markets/%d/orders?order_type=sell", esiBaseURL, regionId)
	pageUrl := func(page int) string { return fmt.Sprintf("%s&page=%d", baseUrl, page) }

	var first []RegionOrder
	header, err := c.do(ctx, "GET", pageUrl(1), "", nil, &first)
	if err != nil {
		return nil, err
	}
	pages := 1
	if totalPagesStr := header.Get("X-Pages"); totalPagesStr != "" {
		if pages, err = strconv.Atoi(totalPagesStr); err != nil {
			return nil, fmt.Errorf("invalid X-Pages header %q from %s: %w", totalPagesStr, baseUrl, err)
		}
	}

	results := make([][]RegionOrder, pages+1)
	results[1] = first

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	next := make(chan int)
	var wg sync.WaitGroup
	var errOnce sync.Once
	var firstErr error
	for range min(regionOrderWorkers, pages-1) {
		wg.Go(func() {
			for page := range next {
				var orders []RegionOrder
				if _, err := c.do(ctx, "GET", pageUrl(page), "", nil, &orders); err != nil {
					errOnce.Do(func() { firstErr = err; cancel() })
					continue
				}
				results[page] = orders
			}
		})
	}
feed:
	for page := 2; page <= pages; page++ {
		select {
		case next <- page:
		case <-ctx.Done():
			break feed
		}
	}
	close(next)
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	seen := map[int64]bool{}
	var all []RegionOrder
	for _, page := range results {
		for _, order := range page {
			if !order.IsBuyOrder && !seen[order.OrderId] {
				seen[order.OrderId] = true
				all = append(all, order)
			}
		}
	}
	return all, nil
}

// GetAveragePrices returns CCP's average traded price of each type, from
// /markets/prices, keyed by type ID. Types with no average are left out.
func (c *Client) GetAveragePrices(ctx context.Context) (map[int64]float64, error) {
	var prices []struct {
		TypeId       int64   `json:"type_id"`
		AveragePrice float64 `json:"average_price"`
	}
	if _, err := c.do(ctx, "GET", esiBaseURL+"/markets/prices", "", nil, &prices); err != nil {
		return nil, err
	}
	averages := make(map[int64]float64, len(prices))
	for _, p := range prices {
		if p.AveragePrice > 0 {
			averages[p.TypeId] = p.AveragePrice
		}
	}
	return averages, nil
}

// GetCharacterWallet needs the esi-wallet.read_character_wallet.v1 scope.
func (c *Client) GetCharacterWallet(ctx context.Context, characterId int64, accessToken string) (float64, error) {
	var balance float64
	_, err := c.do(ctx, "GET", fmt.Sprintf("%s/characters/%d/wallet", esiBaseURL, characterId), accessToken, nil, &balance)
	return balance, err
}

// CorporationWallet is one of a corporation's seven wallet divisions.
type CorporationWallet struct {
	Division int     `json:"division"`
	Balance  float64 `json:"balance"`
}

// GetCorporationWallets needs the esi-wallet.read_corporation_wallets.v1
// scope and the Accountant, Junior Accountant or Director role.
func (c *Client) GetCorporationWallets(ctx context.Context, corporationId int64, accessToken string) ([]CorporationWallet, error) {
	var wallets []CorporationWallet
	_, err := c.do(ctx, "GET", fmt.Sprintf("%s/corporations/%d/wallets", esiBaseURL, corporationId), accessToken, nil, &wallets)
	return wallets, err
}
