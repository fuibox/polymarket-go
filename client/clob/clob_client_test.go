package clob

import (
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/shopspring/decimal"
	"github.com/fuibox/polymarket-go/client/clob/clob_types"
	config2 "github.com/fuibox/polymarket-go/client/config"
	"github.com/fuibox/polymarket-go/client/constants"
	"github.com/fuibox/polymarket-go/client/relayer/builder"
	"github.com/fuibox/polymarket-go/client/signer"
	"github.com/fuibox/polymarket-go/client/types"
	"github.com/fuibox/polymarket-go/tools/headers"
	"github.com/fuibox/polymarket-go/turnkey"
)

func initClobClientWithTurnkey(turnkeyAccount common.Address) (client *ClobClient, safeAddr *common.Address, err error) {
	builderConfig := &headers.BuilderConfig{
		APIKey:     "YOUR_BUILDER_API_KEY",
		Secret:     "YOUR_BUILDER_SECRET",
		Passphrase: "YOUR_BUILDER_PASSPHRASE",
	}
	apiCreds := &types.ApiKeyCreds{}

	turnkeyConfig := turnkey.Config{
		PubKey:       "YOUR_TURNKEY_API_PUBLIC_KEY",
		PrivateKey:   "YOUR_TURNKEY_API_PRIVATE_KEY",
		Organization: "YOUR_TURNKEY_ORGANIZATION_ID",
		WalletName:   "YOUR_TURNKEY_WALLET_NAME",
	}
	signerConfig := signer.SignerConfig{
		SignerType:    signer.Turnkey,
		ChainID:       137,
		TurnkeyConfig: &turnkeyConfig,
	}
	signerHandler, err := signer.NewSigner(signerConfig)
	if err != nil {
		log.Fatal(err)
	}
	config := &ClientConfig{
		Host:          "https://clob.polymarket.com",
		ChainID:       types.ChainPolygon,
		Signer:        signerHandler,
		APIKey:        apiCreds,
		UseServerTime: true,
		Timeout:       30 * time.Second,
		ProxyUrl:      "",
		BuilderConfig: builderConfig,
	}

	clobClient, err := NewClobClient(config)
	if err != nil {
		return nil, nil, err
	}
	fmt.Println("CLOB client created successfully")

	serverTime, err := clobClient.GetServerTime()
	if err != nil {
		return nil, nil, err
	} else {
		fmt.Printf("Server time: %d\n", serverTime)
	}

	fmt.Println("Creating API key...")
	option := clob_types.ClobOption{
		TurnkeyAccount: turnkeyAccount,
	}

	contractConfig, err := config2.GetContractConfig(types.Chain(signerHandler.ChainID()))
	if err != nil {
		return nil, nil, err
	}
	option.SafeAccount = builder.Derive(option.TurnkeyAccount, contractConfig.SafeFactory)
	log.Printf("safe: %v\n", option.SafeAccount.Hex())
	apiCreds, err = clobClient.CreateOrDeriveApiKey(nil, option)
	if err != nil {
		return nil, nil, err
	}
	config.APIKey = apiCreds
	clobClient, err = NewClobClient(config)

	return clobClient, &option.SafeAccount, nil
}

func initClobClientWithPrivateKey() (client *ClobClient, safeAddr *common.Address, err error) {
	pubKey := "YOUR_EOA_ADDRESS"         // e.g. "0x..."
	privateKey := "YOUR_PRIVATE_KEY_HEX" // hex without 0x prefix
	priKey, err := crypto.HexToECDSA(privateKey)
	if err != nil {
		log.Fatalf("[trade] Failed to parse private key: %v", err)
	}
	builderConfig := &headers.BuilderConfig{
		APIKey:     "YOUR_BUILDER_API_KEY",
		Secret:     "YOUR_BUILDER_SECRET",
		Passphrase: "YOUR_BUILDER_PASSPHRASE",
	}
	apiCreds := &types.ApiKeyCreds{}

	signerConfig := signer.SignerConfig{
		SignerType: signer.PrivateKey,
		ChainID:    137,
		PrivateKeyConfig: &signer.PrivateKeyClient{
			PrivateKey: priKey,
			Address:    common.HexToAddress(pubKey),
		},
	}
	signerHandler, err := signer.NewSigner(signerConfig)
	if err != nil {
		log.Fatal(err)
	}
	config := &ClientConfig{
		Host:          "https://clob.polymarket.com",
		ChainID:       types.ChainPolygon,
		Signer:        signerHandler,
		APIKey:        apiCreds,
		UseServerTime: true,
		Timeout:       30 * time.Second,
		ProxyUrl:      "http://127.0.0.1:7890",
		BuilderConfig: builderConfig,
	}

	clobClient, err := NewClobClient(config)
	if err != nil {
		return nil, nil, err
	}
	fmt.Println("CLOB client created successfully")

	serverTime, err := clobClient.GetServerTime()
	if err != nil {
		return nil, nil, err
	} else {
		fmt.Printf("Server time: %d\n", serverTime)
	}

	option := clob_types.ClobOption{}

	contractConfig, err := config2.GetContractConfig(types.Chain(signerHandler.ChainID()))
	if err != nil {
		return nil, nil, err
	}
	option.SafeAccount = builder.Derive(common.HexToAddress(pubKey), contractConfig.SafeFactory)

	log.Printf("safe: %v\n", option.SafeAccount.Hex())

	apiCreds, err = clobClient.CreateOrDeriveApiKey(nil, option)
	if err != nil {
		return nil, nil, err
	}
	config.APIKey = apiCreds
	clobClient, err = NewClobClient(config)

	return clobClient, &option.SafeAccount, nil
}

func TestClobClient_GetPrice(t *testing.T) {
	tokenId := ""
	config := &ClientConfig{
		Host:          "https://clob.polymarket.com",
		ChainID:       types.ChainPolygon,
		Signer:        nil,
		APIKey:        nil,
		UseServerTime: true,
		Timeout:       30 * time.Second,
		ProxyUrl:      "",
	}

	clobClient, err := NewClobClient(config)
	if err != nil {
		t.Fatal(err)
	}
	res, err := clobClient.GetPrice(tokenId, types.SideBuy)
	if err != nil {
		t.Fatal(err)
	}
	log.Printf("res: %v", res)

}

func Test_CreateClobClient(t *testing.T) {
	pubKey := ""
	privateKey := ""
	builderConfig := &headers.BuilderConfig{
		APIKey:     "",
		Secret:     "",
		Passphrase: "",
	}

	apiCreds := &types.ApiKeyCreds{}
	priKey, err := crypto.HexToECDSA(privateKey)
	if err != nil {
		log.Fatal(err)
	}
	privateKeyConfig := signer.PrivateKeyClient{
		Address:    common.HexToAddress(pubKey),
		PrivateKey: priKey,
	}
	signerConfig := signer.SignerConfig{
		SignerType:       signer.PrivateKey,
		ChainID:          137,
		PrivateKeyConfig: &privateKeyConfig,
	}
	signerHandler, err := signer.NewSigner(signerConfig)
	if err != nil {
		log.Fatal(err)
	}
	config := &ClientConfig{
		Host:          "https://clob.polymarket.com",
		ChainID:       types.ChainPolygon, // 137 for Polygon
		Signer:        signerHandler,
		APIKey:        apiCreds,
		UseServerTime: true,
		Timeout:       30 * time.Second,
		ProxyUrl:      "",
		BuilderConfig: builderConfig,
	}

	clobClient, err := NewClobClient(config)
	if err != nil {
		t.Errorf("failed to create clobClient: %v", err)
	}
	fmt.Println("CLOB client created successfully")

	serverTime, err := clobClient.GetServerTime()
	if err != nil {
		t.Errorf("failed to get server time: %v", err)
	} else {
		fmt.Printf("Server time: %d\n", serverTime)
	}

	fmt.Println("Creating API key...")
	option := clob_types.ClobOption{}

	apiKey, err := clobClient.CreateApiKey(nil, option)
	if err != nil {
		log.Printf("Failed to create API key: %v", err)
		fmt.Println("Note: This might fail if you already have an API key")
		log.Printf("Start derive API creds ... \n")
		apiCreds, err = clobClient.DeriveApiKey(nil, option)
		if err != nil {
			log.Printf("Failed to derive API creds: %v", err)
		}
		log.Printf("derive API creds: %v\n", apiCreds)
		config.APIKey = apiCreds
		clobClient, err = NewClobClient(config)
		if err != nil {
			t.Errorf("failed to create clobClient: %v", err)
		}
	} else {
		fmt.Printf("API Key created successfully:\n")
		fmt.Printf("  Key: %s\n", apiKey.Key)
		fmt.Printf("  (Secret and passphrase are sensitive)\n")

		apiCreds = apiKey
		config.APIKey = apiCreds

		clobClient, err = NewClobClient(config)
		if err != nil {
			t.Errorf("failed to create clobClient: %v", err)
		}
	}
	funder := common.HexToAddress(signerHandler.Address())

	if apiCreds != nil {

		apiKeys, err := clobClient.GetApiKeys(funder)
		if err != nil {
			t.Errorf("failed to get api keys: %v", err)
		} else {
			fmt.Printf("Found %d API keys\n", len(apiKeys.APIKeys))
		}

		banStatus, err := clobClient.GetClosedOnlyMode(funder)
		if err != nil {
			t.Errorf("failed to get closed only mode: %v", err)
		} else {
			fmt.Printf("Closed only mode: %v\n", banStatus.ClosedOnly)
		}

		tradesResp, err := clobClient.GetTrades(funder, nil, "")
		if err != nil {
			t.Errorf("failed to get trades: %v", err)
		} else {
			fmt.Printf("Found %d trades\n", len(tradesResp.Data))
		}
	}

}

func Test_CreateClobClientWithTurnkey(t *testing.T) {
	builderConfig := &headers.BuilderConfig{
		APIKey:     "",
		Secret:     "",
		Passphrase: "",
	}

	apiCreds := &types.ApiKeyCreds{}
	turnkeyConfig := turnkey.Config{
		PubKey:       "",
		PrivateKey:   "",
		Organization: "",
		WalletName:   "",
	}
	signerConfig := signer.SignerConfig{
		SignerType:    signer.Turnkey,
		ChainID:       137,
		TurnkeyConfig: &turnkeyConfig,
	}
	signerHandler, err := signer.NewSigner(signerConfig)
	if err != nil {
		log.Fatal(err)
	}
	config := &ClientConfig{
		Host:          "https://clob.polymarket.com",
		ChainID:       types.ChainPolygon, // 137 for Polygon
		Signer:        signerHandler,
		UseServerTime: true,
		Timeout:       30 * time.Second,
		ProxyUrl:      "",
		BuilderConfig: builderConfig,
	}

	clobClient, err := NewClobClient(config)
	if err != nil {
		t.Errorf("failed to create clobClient: %v", err)
	}
	fmt.Println("CLOB client created successfully")

	serverTime, err := clobClient.GetServerTime()
	if err != nil {
		t.Errorf("failed to get server time: %v", err)
	} else {
		fmt.Printf("Server time: %d\n", serverTime)
	}

	fmt.Println("Creating API key...")
	option := clob_types.ClobOption{
		TurnkeyAccount: common.HexToAddress(""),
	}
	contractConfig, err := config2.GetContractConfig(types.Chain(signerHandler.ChainID()))
	if err != nil {
		t.Errorf("failed to get contract config: %v", err)
	}
	option.SafeAccount = builder.Derive(option.TurnkeyAccount, contractConfig.SafeFactory)
	log.Printf("safe: %v\n", option.SafeAccount.Hex())
	apiKey, err := clobClient.CreateApiKey(nil, option)
	if err != nil {
		log.Printf("Failed to create API key: %v", err)
		fmt.Println("Note: This might fail if you already have an API key")
		log.Printf("Start derive API creds ... \n")
		apiCreds, err = clobClient.DeriveApiKey(nil, option)
		if err != nil {
			log.Printf("Failed to derive API creds: %v", err)
			return
		}
		fmt.Printf("api  key: %s\n", apiCreds.Key)
		fmt.Printf("api secret: %s\n", apiCreds.Secret)
		fmt.Printf("api passphrase:%s\n", apiCreds.Passphrase)
		config.APIKey = apiCreds
		clobClient, err = NewClobClient(config)
		if err != nil {
			t.Errorf("failed to create clobClient: %v", err)
		}
	} else {
		fmt.Printf("API Key created successfully:\n")
		fmt.Printf("api  key: %s\n", apiKey.Key)
		fmt.Printf("api secret: %s\n", apiKey.Secret)
		fmt.Printf("api passphrase:%s\n", apiKey.Passphrase)

		apiCreds = apiKey
		config.APIKey = apiCreds

		clobClient, err = NewClobClient(config)
		if err != nil {
			t.Errorf("failed to create clobClient: %v", err)
		}
	}
	addr := option.TurnkeyAccount

	if config.APIKey != nil {
		apiKeys, err := clobClient.GetApiKeys(addr)
		if err != nil {
			t.Errorf("failed to get api keys: %v", err)
			return
		} else {
			fmt.Printf("Found %d API keys\n", len(apiKeys.APIKeys))
		}

		banStatus, err := clobClient.GetClosedOnlyMode(addr)
		if err != nil {
			t.Errorf("failed to get closed only mode: %v", err)
			return
		} else {
			fmt.Printf("Closed only mode: %v\n", banStatus.ClosedOnly)
		}

		tradesResp, err := clobClient.GetTrades(addr, nil, "")
		if err != nil {
			t.Errorf("failed to get trades: %v", err)
		} else {
			fmt.Printf("Found %d trades\n", len(tradesResp.Data))
		}
	}
}

func Test_TurnkeyCreateOrder(t *testing.T) {
	turnkeyAccount := common.HexToAddress("YOUR_TURNKEY_EOA_ADDRESS")
	clobClient, safeAddr, err := initClobClientWithTurnkey(turnkeyAccount)
	if err != nil {
		t.Errorf("failed to init clobClient: %v", err)
		return
	}
	if safeAddr == nil || *safeAddr == constants.ZERO_ADDRESS {
		t.Errorf("invalid safeAddr")
		return
	}
	price, err := decimal.NewFromString("0.5")
	size, err := decimal.NewFromString("5")
	args := clob_types.OrderArgs{
		TokenID:    "YOUR_TOKEN_ID",
		Price:      price,
		Size:       size,
		Side:       "BUY",
		FeeRateBps: 0,
		Nonce:      0,
		Expiration: 0,
		Taker:      constants.ZERO_ADDRESS,
	}
	orderOption := clob_types.PartialCreateOrderOptions{
		OrderType:      types.OrderTypeGTC,
		TurnkeyAccount: turnkeyAccount,
		SafeAccount:    *safeAddr,
	}
	resp, err := clobClient.CreateAndPostOrder(args, orderOption)
	if err != nil {
		t.Errorf("failed to create order: %v", err)
		return
	}
	str, err := sonic.MarshalString(resp)
	if err != nil {
		t.Errorf("failed to marshal response: %v", err)
		return
	}
	fmt.Println(str)
}

// Test_PrivateKeyCreateOrder creates a limit order using private key signing.
// Uses initClobClientWithPrivateKey to initialize the client and derive the safe address.
func Test_PrivateKeyCreateOrder(t *testing.T) {
	clobClient, safe, err := initClobClientWithPrivateKey()
	if err != nil {
		t.Errorf("failed to init clobClient: %v", err)
		return
	}
	if safe == nil || *safe == constants.ZERO_ADDRESS {
		t.Errorf("invalid safeAddr")
		return
	}
	price, err := decimal.NewFromString("0.51")
	size, err := decimal.NewFromString("5")
	args := clob_types.OrderArgs{
		TokenID:    "YOUR_TOKEN_ID",
		Price:      price,
		Size:       size,
		Side:       "SELL",
		FeeRateBps: 0,
		Nonce:      0,
		Expiration: 0,
		Taker:      constants.ZERO_ADDRESS,
	}
	orderOption := clob_types.PartialCreateOrderOptions{
		OrderType:   types.OrderTypeGTC,
		SafeAccount: *safe,
	}
	resp, err := clobClient.CreateAndPostOrder(args, orderOption)
	if err != nil {
		t.Errorf("failed to create order: %v", err)
		return
	}
	str, err := sonic.MarshalString(resp)
	if err != nil {
		t.Errorf("failed to marshal response: %v", err)
		return
	}
	fmt.Println(str)
}

func Test_TurnkeyCreateMarketOrder(t *testing.T) {
	turnkeyAccount := common.HexToAddress("YOUR_TURNKEY_EOA_ADDRESS")
	clobClient, safe, err := initClobClientWithTurnkey(turnkeyAccount)
	if err != nil {
		t.Errorf("failed to create clobClient: %v", err)
		return
	}
	price, err := decimal.NewFromString("0.1")
	amount := decimal.NewFromFloat(3.235293)
	args := clob_types.MarketOrderArgs{
		TokenID:    "YOUR_TOKEN_ID",
		Price:      price,
		Amount:     amount,
		Side:       "BUY",
		FeeRateBps: 0,
		Nonce:      0,

		Taker: constants.ZERO_ADDRESS,
	}
	orderOption := clob_types.PartialCreateOrderOptions{
		OrderType:      types.OrderTypeFOK,
		TurnkeyAccount: turnkeyAccount,
		SafeAccount:    *safe,
	}
	resp, err := clobClient.CreateAndPostMarketOrder(args, orderOption)
	if err != nil {
		t.Errorf("failed to create order: %v", err)
		return
	}
	str, err := sonic.MarshalString(resp)
	if err != nil {
		t.Errorf("failed to marshal response: %v", err)
		return
	}

	fmt.Println(str)
}

func Test_PrivateKeyCreateMarketOrder(t *testing.T) {

	clobClient, safe, err := initClobClientWithPrivateKey()
	if err != nil {
		t.Errorf("failed to create clobClient: %v", err)
		return
	}
	price, err := decimal.NewFromString("0.99")
	amount := decimal.NewFromFloat(4.8)
	args := clob_types.MarketOrderArgs{
		TokenID:    "YOUR_TOKEN_ID",
		Price:      price,
		Amount:     amount,
		Side:       "SELL",
		FeeRateBps: 0,
		Nonce:      0,

		Taker: constants.ZERO_ADDRESS,
	}
	orderOption := clob_types.PartialCreateOrderOptions{
		OrderType:   types.OrderTypeFOK,
		SafeAccount: *safe,
	}
	resp, err := clobClient.CreateAndPostMarketOrder(args, orderOption)
	if err != nil {
		t.Errorf("failed to create order: %v", err)
		return
	}
	str, err := sonic.MarshalString(resp)
	if err != nil {
		t.Errorf("failed to marshal response: %v", err)
		return
	}

	fmt.Println(str)
}

func TestClobClient_GetTrades(t *testing.T) {
	turnkeyAccount := common.HexToAddress("")
	clobClient, _, err := initClobClientWithTurnkey(turnkeyAccount)
	if err != nil {
		t.Errorf("failed to create clobClient: %v", err)
		return
	}
	tradesResp, err := clobClient.GetTrades(turnkeyAccount, nil, "")
	if err != nil {
		t.Errorf("failed to get trades: %v", err)
	} else {
		fmt.Printf("Found %d trades, nextCursor: %s\n", len(tradesResp.Data), tradesResp.NextCursor)
		tradesStr, err := sonic.MarshalString(tradesResp.Data)
		if err != nil {
			t.Errorf("failed to marshal trades: %v", err)
			return
		}
		fmt.Printf("trades: %s\n", tradesStr)
	}
}

// TestClobClient_GetTradesWithPrivateKey queries trade history using private key signing.
// The funder address must be the EOA address used to derive the API key.
func TestClobClient_GetTradesWithPrivateKey(t *testing.T) {
	eoaAddr := common.HexToAddress("YOUR_EOA_ADDRESS")
	clobClient, _, err := initClobClientWithPrivateKey()
	if err != nil {
		t.Errorf("failed to create clobClient: %v", err)
		return
	}
	tradesResp, err := clobClient.GetTrades(eoaAddr, nil, "")
	if err != nil {
		t.Errorf("failed to get trades: %v", err)
		return
	}
	fmt.Printf("Found %d trades, nextCursor: %s\n", len(tradesResp.Data), tradesResp.NextCursor)
	tradesStr, err := sonic.MarshalString(tradesResp.Data)
	if err != nil {
		t.Errorf("failed to marshal trades: %v", err)
		return
	}
	fmt.Printf("trades: %s\n", tradesStr)
}

func TestClobClient_GetOrder(t *testing.T) {
	turnkeyAccount := common.HexToAddress("YOUR_TURNKEY_EOA_ADDRESS")
	orderId := "YOUR_ORDER_ID"
	clobClient, _, err := initClobClientWithTurnkey(turnkeyAccount)
	if err != nil {
		t.Errorf("failed to create clobClient: %v", err)
		return
	}
	order, err := clobClient.GetOrder(turnkeyAccount, orderId)
	if err != nil {
		t.Errorf("failed to get order: %v", err)
		return
	}
	orderStr, err := sonic.MarshalString(order)
	if err != nil {
		t.Errorf("failed to marshal order: %v", err)
		return
	}
	fmt.Printf("Order: %v\n", orderStr)
}

func TestClobClient_GetOrderWithPrivateKey(t *testing.T) {
	orderId := "YOUR_ORDER_ID"
	eoaAddr := common.HexToAddress("YOUR_EOA_ADDRESS")
	clobClient, _, err := initClobClientWithPrivateKey()
	if err != nil {
		t.Errorf("failed to create clobClient: %v", err)
		return
	}
	order, err := clobClient.GetOrder(eoaAddr, orderId)
	if err != nil {
		t.Errorf("failed to get order: %v", err)
		return
	}
	orderStr, err := sonic.MarshalString(order)
	if err != nil {
		t.Errorf("failed to marshal order: %v", err)
		return
	}
	fmt.Printf("Order: %v\n", orderStr)
}

func TestClobClient_CancelOrder(t *testing.T) {
	turnkeyAccount := common.HexToAddress("YOUR_TURNKEY_EOA_ADDRESS")
	orderId := "YOUR_ORDER_ID"
	clobClient, _, err := initClobClientWithTurnkey(turnkeyAccount)
	if err != nil {
		t.Errorf("failed to create clobClient: %v", err)
		return
	}
	res, err := clobClient.CancelOrder(orderId, turnkeyAccount)
	if err != nil {
		t.Errorf("failed to cancel order: %v", err)
		return
	}
	resStr, err := sonic.MarshalString(res)
	if err != nil {
		t.Errorf("failed to marshal order: %v", err)
		return
	}
	fmt.Printf("Order resp: %v\n", resStr)
}

func TestClobClient_CancelOrderWithPrivateKey(t *testing.T) {
	signerAddr := common.HexToAddress("YOUR_EOA_ADDRESS")
	orderId := "YOUR_ORDER_ID"
	clobClient, _, err := initClobClientWithPrivateKey()
	if err != nil {
		t.Errorf("failed to create clobClient: %v", err)
		return
	}
	res, err := clobClient.CancelOrder(orderId, signerAddr)
	if err != nil {
		t.Errorf("failed to cancel order: %v", err)
		return
	}
	resStr, err := sonic.MarshalString(res)
	if err != nil {
		t.Errorf("failed to marshal order: %v", err)
		return
	}
	fmt.Printf("Order resp: %v\n", resStr)
}

// TestClobClient_CancelAllOrdersWithPrivateKey cancels all open orders for the EOA.
// The signerAddr must be the EOA address used to derive the API key.
func TestClobClient_CancelAllOrdersWithPrivateKey(t *testing.T) {
	eoaAddr := common.HexToAddress("YOUR_EOA_ADDRESS")
	clobClient, _, err := initClobClientWithPrivateKey()
	if err != nil {
		t.Errorf("failed to create clobClient: %v", err)
		return
	}
	res, err := clobClient.CancelAllOrders(eoaAddr)
	if err != nil {
		t.Errorf("failed to cancel all orders: %v", err)
		return
	}
	resStr, err := sonic.MarshalString(res)
	if err != nil {
		t.Errorf("failed to marshal result: %v", err)
		return
	}
	fmt.Printf("Cancel all orders resp: %v\n", resStr)
}

func TestGetPrice(t *testing.T) {
	turnkeyAccount := common.HexToAddress("")
	tokenId := ""
	side := types.SideBuy
	clobClient, _, err := initClobClientWithTurnkey(turnkeyAccount)
	if err != nil {
		t.Errorf("failed to create clobClient: %v", err)
		return
	}
	res, err := clobClient.GetPrice(tokenId, side)
	if err != nil {
		t.Errorf("failed to get price: %v", err)
		return
	}
	log.Printf("Price: %v\n", res.String())
}

func TestClobClient_GetTickSize(t *testing.T) {
	tokenId := ""
	turnkeyAccount := common.HexToAddress("")
	clobClient, _, err := initClobClientWithTurnkey(turnkeyAccount)
	if err != nil {
		t.Errorf("failed to create clobClient: %v", err)
		return
	}
	tickSize, err := clobClient.GetTickSize(tokenId)
	if err != nil {
		t.Errorf("failed to get tick size: %v", err)
		return
	}
	log.Printf("Tick size: %v\n", tickSize)
}

// TestClobClient_GetApiKeysWithPrivateKey lists all API keys registered under the EOA.
func TestClobClient_GetApiKeysWithPrivateKey(t *testing.T) {
	eoaAddr := common.HexToAddress("YOUR_EOA_ADDRESS")
	clobClient, _, err := initClobClientWithPrivateKey()
	if err != nil {
		t.Errorf("failed to create clobClient: %v", err)
		return
	}
	apiKeys, err := clobClient.GetApiKeys(eoaAddr)
	if err != nil {
		t.Errorf("failed to get api keys: %v", err)
		return
	}
	fmt.Printf("Found %d API keys\n", len(apiKeys.APIKeys))
	keysStr, err := sonic.MarshalString(apiKeys)
	if err != nil {
		t.Errorf("failed to marshal api keys: %v", err)
		return
	}
	fmt.Printf("API keys: %s\n", keysStr)
}

// TestClobClient_GetBalanceAllowanceWithPrivateKey verifies the GetBalanceAllowance method
// authenticates and parses responses correctly.
//
// Architecture note — proxy wallet mode:
//
//	The PrivateKey signer derives the API key with POLY_ADDRESS = EOA, so all L2 auth
//	must use the EOA address. The actual tokens (USDC / YES / NO) are held by the Gnosis
//	Safe derived from the EOA, NOT the EOA itself.
//
//	Consequence: balance-allowance returns the EOA's on-chain balance, which is 0.
//	Passing the safe address instead causes HTTP 401 ("Invalid api key") because the
//	API key is not registered under the safe address in this auth mode.
//
//	In contrast, Python's py-clob-client proxy-wallet mode derives the API key with
//	POLY_ADDRESS = safe, so get_balance_allowance returns the safe's balance there.
//
//	For split verification in proxy wallet setups, query the safe's ERC-1155 balance
//	directly via a Polygon RPC call instead of using this endpoint.
func TestClobClient_GetBalanceAllowanceWithPrivateKey(t *testing.T) {
	// POLY_ADDRESS must be the EOA — that is what the API key was derived for.
	eoaAddr := common.HexToAddress("YOUR_EOA_ADDRESS")
	clobClient, safeAddr, err := initClobClientWithPrivateKey()
	if err != nil {
		t.Errorf("failed to init clobClient: %v", err)
		return
	}
	log.Printf("EOA: %v  safe: %v\n", eoaAddr.Hex(), safeAddr.Hex())

	// Query USDC (collateral) balance & allowance.
	// Expected: balance="0" because the EOA holds no USDC; tokens are in the safe.
	collateralResp, err := clobClient.GetBalanceAllowance(eoaAddr, &types.BalanceAllowanceParams{
		AssetType: types.AssetTypeCollateral,
	})
	if err != nil {
		t.Errorf("failed to get collateral balance allowance: %v", err)
		return
	}
	collateralStr, _ := sonic.MarshalString(collateralResp)
	fmt.Printf("Collateral (USDC) EOA balance+allowance: %s\n", collateralStr)

	// Query conditional (YES/NO token) balance & allowance.
	// Expected: balance="0" for the same reason.
	tokenId := "YOUR_TOKEN_ID"
	conditionalResp, err := clobClient.GetBalanceAllowance(eoaAddr, &types.BalanceAllowanceParams{
		AssetType: types.AssetTypeConditional,
		TokenID:   &tokenId,
	})
	if err != nil {
		t.Errorf("failed to get conditional balance allowance: %v", err)
		return
	}
	conditionalStr, _ := sonic.MarshalString(conditionalResp)
	fmt.Printf("Conditional token EOA balance+allowance: %s\n", conditionalStr)
}
