package relayer

import (
	"fmt"
	"log"
	"math/big"
	"testing"

	"github.com/bytedance/sonic"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/shopspring/decimal"
	"github.com/fuibox/polymarket-go/client/constants"
	"github.com/fuibox/polymarket-go/client/relayer/builder"
	"github.com/fuibox/polymarket-go/client/signer"
	"github.com/fuibox/polymarket-go/client/types"
	"github.com/fuibox/polymarket-go/tools/headers"
	"github.com/fuibox/polymarket-go/turnkey"
)

func newRelayClient() (*RelayClient, error) {

	turkeyConfig := turnkey.Config{
		PubKey:       "",
		PrivateKey:   "",
		Organization: "",
		WalletName:   "",
	}
	signerConfig := signer.SignerConfig{
		SignerType:       signer.Turnkey,
		PrivateKeyConfig: nil,
		TurnkeyConfig:    &turkeyConfig,
		ChainID:          137,
	}
	s, err := signer.NewSigner(signerConfig)
	if err != nil {
		return nil, err
	}
	relayerUrl := "https://relayer-v2.polymarket.com"
	chainId := types.ChainPolygon

	builderConfig := headers.BuilderConfig{
		APIKey:     "",
		Secret:     "",
		Passphrase: "",
	}
	proxyUrl := ""
	relayClient, err := NewRelayClient(relayerUrl, chainId, s, &builderConfig, &proxyUrl, nil)
	if err != nil {
		return nil, err
	}
	return relayClient, nil
}

func newRelayClientWithPrivateKey() (*RelayClient, error) {
	privateKey := "YOUR_PRIVATE_KEY_HEX" // hex without 0x prefix
	priKey, err := crypto.HexToECDSA(privateKey)
	if err != nil {
		log.Fatal(err)
	}
	publicKey := common.HexToAddress("YOUR_EOA_ADDRESS")
	conf := &signer.PrivateKeyClient{
		PrivateKey: priKey,
		Address:    publicKey,
	}

	signerConfig := signer.SignerConfig{
		SignerType:       signer.PrivateKey,
		PrivateKeyConfig: conf,
		ChainID:          137,
	}
	s, err := signer.NewSigner(signerConfig)
	if err != nil {
		return nil, err
	}
	relayerUrl := "https://relayer-v2.polymarket.com"
	chainId := types.ChainPolygon

	builderConfig := headers.BuilderConfig{
		APIKey:     "YOUR_BUILDER_API_KEY",
		Secret:     "YOUR_BUILDER_SECRET",
		Passphrase: "YOUR_BUILDER_PASSPHRASE",
	}
	proxyUrl := "http://127.0.0.1:7890"
	relayClient, err := NewRelayClient(relayerUrl, chainId, s, &builderConfig, &proxyUrl, nil)
	if err != nil {
		return nil, err
	}
	return relayClient, nil
}

func TestDeriveSafeWithTurnkey(t *testing.T) {
	turnkeyAccount := common.HexToAddress("")

	relayClient, err := newRelayClient()
	if err != nil {
		t.Error(err)
		return
	}
	safe, res, err := relayClient.DeployWithTurnkey(turnkeyAccount)
	if err != nil {
		t.Error(err)
		return
	}
	fmt.Printf("hash:%v \n", res.Hash)
	fmt.Printf("txhash:%v \n", res.TransactionHash)
	fmt.Printf("txId:%v \n", res.TransactionID)
	fmt.Printf("safe:%v \n", safe)
}

func TestDeriveSafeWithPrivateKey(t *testing.T) {

	relayClient, err := newRelayClientWithPrivateKey()
	if err != nil {
		t.Error(err)
		return
	}
	res, err := relayClient.DeployWithPrivateKey()

	if err != nil {
		t.Error(err)
		return
	}
	fmt.Printf("hash:%v \n", res.Hash)
	fmt.Printf("txhash:%v \n", res.TransactionHash)
	fmt.Printf("txId:%v \n", res.TransactionID)
}

func TestRelayClient_ApproveForPolymarket(t *testing.T) {

	turnkeyAccount := common.HexToAddress("")

	relayClient, err := newRelayClient()
	if err != nil {
		t.Error(err)
		return
	}
	resp, err := relayClient.ApproveForPolymarketWithTurnkey(turnkeyAccount)
	if err != nil {
		t.Error(err)
		return
	}
	j, err := sonic.MarshalString(resp)
	if err != nil {
		t.Error(err)
		return
	}
	fmt.Printf("resp:%s \n", j)
}

func TestRelayClient_ApproveForPolymarketWithPrivateKey(t *testing.T) {

	relayClient, err := newRelayClientWithPrivateKey()
	if err != nil {
		t.Error(err)
		return
	}
	resp, err := relayClient.ApproveForPolymarketWithPrivateKey()
	if err != nil {
		t.Error(err)
		return
	}
	j, err := sonic.MarshalString(resp)
	if err != nil {
		t.Error(err)
		return
	}
	fmt.Printf("resp:%s \n", j)
}

func TestRelayClient_GetNonce(t *testing.T) {
	turnkeyAccount := common.HexToAddress("")
	relayClient, err := newRelayClient()
	if err != nil {
		t.Error(err)
		return
	}
	safe := builder.Derive(turnkeyAccount, relayClient.ContractConfig.SafeFactory)
	log.Printf("safe:%v \n", safe.Hex())
	nonce, err := relayClient.GetNonce(safe, "SAFE")
	if err != nil {
		t.Error(err)
		return
	}
	fmt.Printf("nonce:%v \n", nonce)
}

func TestRelayClient_GetSafeNonceOnChain(t *testing.T) {
	turnkeyAccount := common.HexToAddress("")
	relayClient, err := newRelayClient()
	if err != nil {
		t.Error(err)
		return
	}
	safe := builder.Derive(turnkeyAccount, relayClient.ContractConfig.SafeFactory)
	log.Printf("safe:%v \n", safe.Hex())
	nonce, err := relayClient.GetSafeNonceOnChain(safe)
	if err != nil {
		t.Error(err)
		return
	}
	fmt.Printf("nonce:%v \n", nonce)
}

func TestRelayClient_IsDeployed(t *testing.T) {
	turnkeyAccount := common.HexToAddress("")

	relayClient, err := newRelayClient()
	if err != nil {
		t.Error(err)
		return
	}
	safe := builder.Derive(turnkeyAccount, relayClient.ContractConfig.SafeFactory)
	log.Printf("safe:%v \n", safe)
	resp, err := relayClient.IsDeployed(safe)
	if err != nil {
		t.Error(err)
		return
	}
	j, _ := sonic.MarshalString(resp)
	fmt.Printf("resp:%s \n", j)

}
func TestRelayClient_IsDeployedWithPrivateKey(t *testing.T) {
	relayClient, err := newRelayClientWithPrivateKey()
	if err != nil {
		t.Error(err)
		return
	}
	safe := builder.Derive(common.HexToAddress(relayClient.Signer.Address()), relayClient.ContractConfig.SafeFactory)
	log.Printf("safe:%v \n", safe)
	resp, err := relayClient.IsDeployed(safe)
	if err != nil {
		t.Error(err)
		return
	}
	j, _ := sonic.MarshalString(resp)
	fmt.Printf("resp:%s \n", j)

}

func TestRelayClient_CheckAllApprovals(t *testing.T) {
	turnkeyAccount := common.HexToAddress("")

	relayClient, err := newRelayClient()
	if err != nil {
		t.Error(err)
		return
	}
	safe := builder.Derive(turnkeyAccount, relayClient.ContractConfig.SafeFactory)
	log.Printf("safe:%v \n", safe.Hex())
	approved, usdcApprovals, tokenApprovals, err := relayClient.CheckAllApprovals(safe)
	if err != nil {
		t.Error(err)
		return
	}
	fmt.Printf("approved:%v, usdcApprove:%v, tokenApprove:%v \n", approved, usdcApprovals, tokenApprovals)
}

func TestRelayClient_CheckAllApprovalsWithPrivateKey(t *testing.T) {

	relayClient, err := newRelayClientWithPrivateKey()
	if err != nil {
		t.Error(err)
		return
	}
	pubKey, err := relayClient.Signer.GetPubkeyOfPrivateKey()
	if err != nil {
		t.Error(err)
		return
	}
	safe := builder.Derive(pubKey, relayClient.ContractConfig.SafeFactory)
	log.Printf("safe:%v \n", safe.Hex())
	approved, usdcApprovals, tokenApprovals, err := relayClient.CheckAllApprovals(safe)
	if err != nil {
		t.Error(err)
		return
	}
	fmt.Printf("approved:%v, usdcApprove:%v, tokenApprove:%v \n", approved, usdcApprovals, tokenApprovals)
}

func TestRelayClient_CheckUsdcApprovalForSpender(t *testing.T) {
	turnkeyAccount := common.HexToAddress("")

	relayClient, err := newRelayClient()
	if err != nil {
		t.Error(err)
		return
	}
	safe := builder.Derive(turnkeyAccount, relayClient.ContractConfig.SafeFactory)
	log.Printf("safe:%v \n", safe.Hex())
	op := common.HexToAddress("")
	ok, err := relayClient.CheckUsdcApprovalForSpender(safe, op)
	if err != nil {
		t.Error(err)
		return
	}
	fmt.Printf("ok:%v \n", ok)
}

func TestRelayClient_CheckERC1155ApprovalForSpender(t *testing.T) {
	turnkeyAccount := common.HexToAddress("")

	relayClient, err := newRelayClient()
	if err != nil {
		t.Error(err)
		return
	}
	safe := builder.Derive(turnkeyAccount, relayClient.ContractConfig.SafeFactory)
	op := common.HexToAddress("")
	ok, err := relayClient.CheckERC1155ApprovalForSpender(safe, op)
	if err != nil {
		t.Error(err)
		return
	}
	fmt.Printf("ok:%v \n", ok)
}

func TestRelayClient_GetTransaction(t *testing.T) {
	transaction := ""
	relayClient, err := newRelayClient()
	if err != nil {
		t.Error(err)
		return
	}
	getTransaction, err := relayClient.GetTransaction(transaction)
	if err != nil {
		t.Error(err)
		return
	}
	j, _ := sonic.MarshalString(getTransaction)
	fmt.Printf("getTransaction:%v \n", j)
}

func TestRelayClient_TransferUsdceFromSafeWithTurnkey(t *testing.T) {
	turnkeyAccount := common.HexToAddress("")
	target := common.HexToAddress("")
	amount := decimal.NewFromFloat(0.12)
	relayClient, err := newRelayClient()
	if err != nil {
		t.Error(err)
		return
	}
	tx, err := relayClient.TransferUsdceFromSafeWithTurnkey(turnkeyAccount, target, amount)
	if err != nil {
		t.Error(err)
		return
	}
	log.Printf("tx:%v \n", tx)
}

func TestRelayClient_TransferUsdceFromSafeWithPrivateKey(t *testing.T) {
	target := common.HexToAddress("YOUR_TARGET_ADDRESS")
	amount := decimal.NewFromFloat(13.245299)
	relayClient, err := newRelayClientWithPrivateKey()
	if err != nil {
		t.Error(err)
		return
	}

	tx, err := relayClient.TransferUsdceFromSafeWithPrivateKey(target, amount)
	if err != nil {
		t.Error(err)
		return
	}
	log.Printf("tx:%v \n", tx)
}

func TestRelayClient_GetNonceFromChain(t *testing.T) {
	turnkeyAccount := common.HexToAddress("")
	relayClient, err := newRelayClient()
	if err != nil {
		t.Error(err)
		return
	}
	safe := builder.Derive(turnkeyAccount, relayClient.ContractConfig.SafeFactory)
	log.Printf("safe:%v \n", safe.Hex())
	nonce, err := relayClient.GetSafeNonceOnChain(safe)
	if err != nil {
		t.Error(err)
		return
	}
	fmt.Printf("nonce:%v \n", nonce)
}

func TestRelayClient_ApproveRequestContractForPolymarket(t *testing.T) {

	turnkeyAccount := common.HexToAddress("")
	contractName := "ctf_exchange"

	relayClient, err := newRelayClient()
	if err != nil {
		t.Error(err)
		return
	}
	resp, err := relayClient.ApproveRequestContractForPolymarketWithTurnkey(turnkeyAccount, contractName)
	if err != nil {
		t.Error(err)
		return
	}
	j, err := sonic.MarshalString(resp)
	if err != nil {
		t.Error(err)
		return
	}
	fmt.Printf("resp:%s \n", j)
}

func TestRelayClient_Split(t *testing.T) {
	relayClient, err := newRelayClientWithPrivateKey()
	if err != nil {
		t.Error(err)
		return
	}
	safe := builder.Derive(common.HexToAddress(relayClient.Signer.Address()), relayClient.ContractConfig.SafeFactory)
	log.Printf("safe:%v \n", safe)

	conditionId := common.HexToHash("0x1835393117ef5f36eb2c74df00685816da5a4acb7c148bedaaf3b9f8910fc87d")
	collateralToken := constants.USDCe
	parentCollectionId := common.Hash{} // zero hash for root collection
	partition := []uint64{1, 2}         // binary market: Yes=1, No=2

	// 3 USDC.e = 3 * 10^6 (6 decimals)
	amount := new(big.Int).Mul(big.NewInt(5), big.NewInt(1e6))

	resp, err := relayClient.SplitPosition(
		constants.ZERO_ADDRESS, // ignored for PrivateKey signer
		collateralToken,
		parentCollectionId,
		conditionId,
		partition,
		amount,
	)
	if err != nil {
		t.Error(err)
		return
	}
	j, err := sonic.MarshalString(resp)
	if err != nil {
		t.Error(err)
		return
	}
	fmt.Printf("resp:%s \n", j)
}
