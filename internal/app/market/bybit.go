package market

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"tarbitrage/pkg/websocket"
	"time"

	"github.com/sirupsen/logrus"
)

type BybitPublicClient struct {
	name   string
	Logger *logrus.Logger
}

type BybitPrivateClient struct {
	name   string
	Key    string
	Secret string
	Logger *logrus.Logger
}

var BybitHost = "https://api.bybit.com"
var BybitPublicWsUrl = "wss://stream.bybit.com/v5/public/spot"

var BybitHeaders = map[string]string{
	"Content-Type":       "application/json",
	"X-BAPI-RECV-WINDOW": "5000",
}

var BybitSides = map[string]string{
	"BUY":  "Buy",
	"SELL": "Sell",
}

type BybitSymbol struct {
	BaseAsset      string
	QuoteAsset     string
	Symbol         string
	BaseSymbol     string
	BasePrecision  int
	PricePrecision int
}

func (s *BybitSymbol) GetBaseAsset() string {
	return s.BaseAsset
}

func (s *BybitSymbol) GetQuoteAsset() string {
	return s.QuoteAsset
}

func (s *BybitSymbol) GetBaseSymbol() string {
	return s.BaseSymbol
}

func (s *BybitSymbol) GetSymbol() string {
	return s.Symbol
}

func (s *BybitSymbol) GetBasePrecision() int {
	return s.BasePrecision
}

func (s *BybitSymbol) GetPricePrecision() int {
	return s.PricePrecision
}

func (s *BybitSymbol) SetBasePrecision(prec int) {
	s.BasePrecision = prec
}

func (s *BybitSymbol) SetPricePrecision(prec int) {
	s.PricePrecision = prec
}

func (c *BybitPublicClient) Name() string {
	return c.name
}

func (c *BybitPrivateClient) Name() string {
	return c.name
}

func (c *BybitPrivateClient) GetKey() string {
	return c.Key
}

func (c *BybitPrivateClient) GetSecret() string {
	return c.Secret
}

func (c *BybitPublicClient) CreateSymbol(base_symbol string) MarketSymbol {
	assets := strings.Split(base_symbol, "+")
	return &BybitSymbol{
		BaseAsset:  assets[0],
		QuoteAsset: assets[1],
		Symbol:     assets[0] + assets[1],
		BaseSymbol: base_symbol,
	}
}

func (c *BybitPublicClient) RunOrderBookStream(symbol MarketSymbol, levels string, handler OrderBookHandler,
	errHandler websocket.ErrHandler) (*websocket.WebSocketApp, error) {

	endpoint := BybitPublicWsUrl
	wsApp := new(websocket.WebSocketApp)
	var snapshotA, snapshotB map[string]string

	type eventData struct {
		Symbol string     `json:"s"`
		Asks   [][]string `json:"a"`
		Bids   [][]string `json:"b"`
	}
	type event struct {
		Type string    `json:"type"`
		Data eventData `json:"data"`
	}
	wsApp.OnOpen = func(ws *websocket.WebSocketApp) {
		if levels != "1" && levels != "50" && levels != "200" {
			levels = "1"
		}
		subscription := map[string]interface{}{
			"req_id": strconv.FormatInt(time.Now().UnixMilli(), 10),
			"op":     "subscribe",
			"args": []string{
				fmt.Sprintf("orderbook.%s.%s", levels, symbol.GetSymbol()),
			},
		}
		ws.Send(subscription)
		c.Logger.Log(logrus.InfoLevel, fmt.Sprintf("Stream %s %s has been started.\n", c.Name(), symbol.GetBaseSymbol()))
	}
	wsApp.OnMessage = func(ws *websocket.WebSocketApp, message []byte) {
		e := new(event)
		err := json.Unmarshal(message, e)

		if err != nil {
			errHandler(err)
			return
		}

		switch e.Type {
		case "snapshot":

			snapshotA = make(map[string]string)
			snapshotB = make(map[string]string)
			bids := e.Data.Bids
			asks := e.Data.Asks

			for _, price_level := range bids {
				snapshotB[price_level[0]] = price_level[1]
			}

			for _, price_level := range asks {
				snapshotA[price_level[0]] = price_level[1]
			}
		case "delta":
			bids := e.Data.Bids
			asks := e.Data.Asks

			for _, delta := range bids {
				if delta[1] == "0" {
					delete(snapshotB, delta[0])
				} else {
					snapshotB[delta[0]] = delta[1]
				}
			}
			for _, delta := range asks {
				if delta[1] == "0" {
					delete(snapshotA, delta[0])
				} else {
					snapshotA[delta[0]] = delta[1]
				}
			}
		default:
			return
		}
		book := new(OrderBookEvent)
		book.Symbol = symbol.GetBaseSymbol()
		book.Asks = make([]PriceLevel, 0)
		book.Bids = make([]PriceLevel, 0)
		for p, q := range snapshotA {
			price, _ := strconv.ParseFloat(p, 64)
			quantity, _ := strconv.ParseFloat(q, 64)
			book.Asks = append(book.Asks, PriceLevel{Price: price, Quantity: quantity})
		}
		for p, q := range snapshotB {
			price, _ := strconv.ParseFloat(p, 64)
			quantity, _ := strconv.ParseFloat(q, 64)
			book.Bids = append(book.Bids, PriceLevel{Price: price, Quantity: quantity})
		}

		handler(book)
	}
	wsApp.OnError = func(ws *websocket.WebSocketApp, err error) {
		c.Logger.Log(logrus.InfoLevel, "STREAM ERROR", err)
	}
	wsApp.OnClose = func(ws *websocket.WebSocketApp) {
		c.Logger.Log(logrus.InfoLevel, fmt.Sprintf("Stream %s %s is stopped.\n", c.Name(), symbol.GetBaseSymbol()))
		if ws.IsRunning {
			ws.Run(ws.Config.Endpoint, ws.Config.WebsocketKeepalive, ws.Config.Timeout)
		}
	}

	if err := wsApp.Run(endpoint, false, 0); err != nil {
		return nil, err
	}

	return wsApp, nil
}

func (c *BybitPublicClient) Perform(query map[string]interface{}, method string, session_method string, result interface{}) error {
	keys := make([]string, 0, len(query))
	for k := range query {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Create a new map with sorted keys
	sortedQuery := make(map[string]interface{})
	for _, k := range keys {
		sortedQuery[k] = query[k]
	}

	// Convert float values to int if possible
	for k, v := range sortedQuery {
		if f, ok := v.(float64); ok && f == float64(int(f)) {
			sortedQuery[k] = int(f)
		}
	}

	var queryStrings []string
	for k, v := range sortedQuery {
		if v != nil {
			queryStrings = append(queryStrings, fmt.Sprintf("%s=%v", k, v))
		}
	}
	queryString := strings.Join(queryStrings, "&")

	url := fmt.Sprintf("%s/%s?%s", BybitHost, method, queryString)

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest(session_method, url, nil)

	if err != nil {
		return err
	}

	for key, value := range BybitHeaders {
		req.Header.Set(key, value)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	res, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	err = json.Unmarshal(res, &result)
	if err != nil {
		return err
	}

	return nil
}

func (c *BybitPublicClient) GetInstrumentsInfo(symbols []MarketSymbol) error {

	parameters := map[string]interface{}{
		"category": "spot",
		"limit":    1000,
	}

	type filter struct {
		BasePrecision string `json:"basePrecision"`
		TickSize      string `json:"tickSize"`
	}

	type SymbolData struct {
		Symbol        string `json:"symbol"`
		Margin        string `json:"marginTrading"`
		PriceFilter   filter `json:"priceFilter"`
		LotSizeFilter filter `json:"lotSizeFilter"`
	}

	type result struct {
		List [1000]SymbolData `json:"list"`
	}

	type response struct {
		Message string `json:"retMsg"`
		Code    int    `json:"retCode"`
		Result  result `json:"result"`
	}

	resp := new(response)

	if err := c.Perform(parameters, "/v5/market/instruments-info", "GET", resp); err != nil {
		return err
	}

	if resp.Code != 0 {
		return fmt.Errorf("bybit error: code: %d, message: %s", resp.Code, resp.Message)
	}

	res := resp.Result.List

	findSymbol := func(data [1000]SymbolData, symbol string) (SymbolData, bool) {
		for _, elm := range data {
			if elm.Symbol == symbol {
				return elm, true
			}
		}
		return SymbolData{}, false
	}

	for _, s := range symbols {
		data, ok := findSymbol(res, s.GetSymbol())
		if !ok {
			return fmt.Errorf("can't find precision information for symbol %s", s.GetBaseSymbol())
		}
		s.SetBasePrecision(GetPrecision(data.LotSizeFilter.BasePrecision))
		s.SetPricePrecision(GetPrecision(data.PriceFilter.TickSize))
	}

	return nil
}

func PreparePayload(method string, parameters map[string]interface{}) (string, error) {
	// Function to cast values to specific types
	castValues := func() error {

		for key, value := range parameters {
			switch key {
			case "qty", "price", "triggerPrice", "takeProfit", "stopLoss":
				if _, ok := value.(string); !ok {
					parameters[key] = fmt.Sprintf("%v", value)
				}
			case "positionIdx":
				if _, ok := value.(int); !ok {
					i, err := strconv.Atoi(fmt.Sprintf("%v", value))
					if err != nil {
						return fmt.Errorf("positionIdx must be number")
					}
					parameters[key] = i
				}
			}
		}

		return nil
	}

	if method == "GET" {
		// Sort keys and construct URL-encoded query string
		keys := make([]string, 0, len(parameters))
		for k := range parameters {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		var queryStrings []string
		for _, k := range keys {
			if v := parameters[k]; v != nil {
				queryStrings = append(queryStrings, fmt.Sprintf("%s=%v", k, v))
			}
		}
		return strings.Join(queryStrings, "&"), nil
	} else {
		if err := castValues(); err != nil {
			return "", err
		}
		payload, err := json.Marshal(parameters)
		if err != nil {
			return "", err
		}
		return string(payload), nil
	}
}

func (c *BybitPrivateClient) generateSignature(queryString string, timestamp int64) (string, error) {
	// Combine data for signing
	recvWindow, _ := strconv.Atoi(BybitHeaders["X-BAPI-RECV-WINDOW"])
	data := fmt.Sprintf("%d%s%d%s", timestamp, c.Key, recvWindow, queryString)

	// Create HMAC with SHA256
	h := hmac.New(sha256.New, []byte(c.Secret))
	_, err := h.Write([]byte(data))
	if err != nil {
		return "", err
	}

	// Generate signature as hex string
	signature := hex.EncodeToString(h.Sum(nil))
	return signature, nil
}

func (c *BybitPrivateClient) PerformSign(query map[string]interface{}, method string, session_method string, result interface{}) error {
	keys := make([]string, 0, len(query))
	for k := range query {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Create a new map with sorted keys
	sortedQuery := make(map[string]interface{})
	for _, k := range keys {
		sortedQuery[k] = query[k]
	}

	// Convert float values to int if possible
	for k, v := range sortedQuery {
		if f, ok := v.(float64); ok && f == float64(int(f)) {
			sortedQuery[k] = int(f)
		}
	}

	query_string, err := PreparePayload(session_method, sortedQuery)
	if err != nil {
		return err
	}

	timestamp := time.Now().UnixMilli()
	signature, err := c.generateSignature(query_string, timestamp)
	if err != nil {
		return err
	}

	BybitHeaders["X-BAPI-API-KEY"] = c.Key
	BybitHeaders["X-BAPI-SIGN"] = signature
	BybitHeaders["X-BAPI-SIGN-TYPE"] = "2"
	BybitHeaders["X-BAPI-TIMESTAMP"] = strconv.FormatInt(timestamp, 10)

	var url string
	var reqBody []byte

	if session_method == "GET" {
		url = fmt.Sprintf("%s/%s?%s", BybitHost, method, query_string)
	} else {
		url = fmt.Sprintf("%s/%s", BybitHost, method)
		reqBody = []byte(query_string)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest(session_method, url, nil)

	if err != nil {
		return err
	}

	for key, value := range BybitHeaders {
		req.Header.Set(key, value)
	}

	if reqBody != nil {
		req.Body = io.NopCloser(bytes.NewReader(reqBody))
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	res, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	err = json.Unmarshal(res, &result)
	if err != nil {
		return err
	}

	return nil
}

func (c *BybitPrivateClient) ApplyInitial(lot float64) error {

	balance, err := c.GetMarginBalance()
	if err != nil {
		return err
	}
	if balance < lot {
		return fmt.Errorf("you have not enough balance (should be greater than lot)")
	}

	collaterals := make([]string, 0)

	data, err := os.ReadFile("./files/bybit/collateral.json")

	if err != nil {
		return err
	}

	err = json.Unmarshal(data, &collaterals)
	if err != nil {
		return err
	}

	if err := c.SetCollateralCoins(collaterals); err != nil {
		return err
	}
	return nil
}

func (c *BybitPrivateClient) SetCollateralCoins(coins []string) error {

	request := make([]map[string]string, len(coins))
	for idx, coin := range coins {
		param := map[string]string{
			"coin":             coin,
			"collateralSwitch": "ON",
		}
		request[idx] = param
	}
	parameters := map[string]interface{}{
		"request": request,
	}

	type result struct {
		Message string `json:"retMsg"`
		Code    int    `json:"retCode"`
	}

	resp := new(result)

	if err := c.PerformSign(parameters, "/v5/account/set-collateral-switch-batch", "POST", resp); err != nil {
		return err
	}
	if resp.Code != 0 {
		return fmt.Errorf("bybit error: code: %d, message: %s", resp.Code, resp.Message)
	}

	if resp.Message != "SUCCESS" {
		return fmt.Errorf("unsuccessful trial to set collateral coins")
	}

	return nil
}

func (c *BybitPrivateClient) GetMarginBalance() (float64, error) {
	parameters := map[string]interface{}{
		"accountType": "UNIFIED",
	}

	type info struct {
		Balance string `json:"totalAvailableBalance"`
	}

	type list struct {
		List []info `json:"list"`
	}

	type result struct {
		Result  list   `json:"result"`
		Code    int    `json:"retCode"`
		Message string `json:"retMsg"`
	}

	resp := new(result)
	if err := c.PerformSign(parameters, "/v5/account/wallet-balance", "GET", resp); err != nil {
		return 0.0, err
	}
	if resp.Code != 0 {
		return 0.0, fmt.Errorf("bybit error: code: %d, message: %s", resp.Code, resp.Message)
	}

	availableStr := resp.Result.List[0].Balance
	available, _ := strconv.ParseFloat(availableStr, 64)

	return available, nil
}

func (c *BybitPrivateClient) PlaceOrder(symbol, side, t, quantity string) (float64, error) {

	parameters := map[string]interface{}{
		"category":    "spot",
		"symbol":      symbol,
		"side":        BybitSides[side],
		"orderType":   "Market",
		"isLeverage":  1,
		"qty":         quantity,
		"orderLinkId": strconv.FormatInt(time.Now().UnixMilli(), 10),
	}

	switch t {
	case "open":
		parameters["marketUnit"] = "quoteCoin"
	case "close":
		parameters["marketUnit"] = "baseCoin"
	}

	type result struct {
		OrderID       string `json:"orderId"`
		ClientOrderID string `json:"orderLinkId"`
	}

	type response struct {
		Code    int    `json:"retCode"`
		Message string `json:"retMsg"`
		Result  result `json:"result"`
	}

	resp := new(response)

	if err := c.PerformSign(parameters, "/v5/order/create", "POST", resp); err != nil {
		return 0, err
	}

	if resp.Code != 0 {
		return 0, fmt.Errorf("bybit error while creating order: code: %d, message: %s", resp.Code, resp.Message)
	}

	orderData, err := c.GetOrderInfo(symbol, resp.Result.OrderID)
	if err != nil {
		return 0, err
	}

	qty, _ := strconv.ParseFloat(orderData.Quantity, 64)

	return qty, nil
}

func (c *BybitPrivateClient) GetOrderInfo(symbol string, orderID string) (*BybitOrderData, error) {
	parameters := map[string]interface{}{
		"category": "spot",
		"symbol":   symbol,
		"orderId":  orderID,
	}

	type result struct {
		List []BybitOrderData `json:"list"`
	}

	type response struct {
		Code    int    `json:"retCode"`
		Message string `json:"retMsg"`
		Result  result `json:"result"`
	}

	resp := new(response)

	if err := c.PerformSign(parameters, "/v5/order/realtime", "GET", resp); err != nil {
		return nil, err
	}

	if resp.Code != 0 {
		return nil, fmt.Errorf("bybit error while fetching order data: code: %d, message: %s", resp.Code, resp.Message)
	}

	data := resp.Result.List[0]

	return &data, nil
}

type BybitOrderData struct {
	Price         string `json:"price"`
	Quantity      string `json:"qty"`
	Side          string `json:"side"`
	QuoteQuantity string `json:"cumExecValue"`
	Commission    string `json:"cumExecFee"`
}
