package txs

import (
	"crypto/ecdsa"
	"errors"
	"fmt"
	"log"
	"math/big"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	tmKv "github.com/tendermint/tendermint/libs/kv"

	"github.com/cosmos/peggy/cmd/ebrelayer/types"
	ethbridge "github.com/cosmos/peggy/x/ethbridge/types"
)

const (
	nullAddress = "0x0000000000000000000000000000000000000000"
)

// LogLockToEthBridgeClaim parses and packages a LockEvent struct with a validator address in an EthBridgeClaim msg
func LogLockToEthBridgeClaim(valAddr sdk.ValAddress, event *types.LockEvent) (ethbridge.EthBridgeClaim, error) {
	witnessClaim := ethbridge.EthBridgeClaim{}

	// chainID type casting (*big.Int -> int)
	chainID := int(event.EthereumChainID.Int64())

	bridgeContractAddress := ethbridge.NewEthereumAddress(event.BridgeContractAddress.Hex())

	// Sender type casting (address.common -> string)
	sender := ethbridge.NewEthereumAddress(event.From.Hex())

	// Recipient type casting ([]bytes -> sdk.AccAddress)
	recipient, err := sdk.AccAddressFromBech32(string(event.To))
	if err != nil {
		return witnessClaim, err
	}
	if recipient.Empty() {
		return witnessClaim, errors.New("empty recipient address")
	}

	// Sender type casting (address.common -> string)
	tokenContractAddress := ethbridge.NewEthereumAddress(event.Token.Hex())

	// Symbol formatted to lowercase
	symbol := strings.ToLower(event.Symbol)
	if symbol == "eth" && !isZeroAddress(event.Token) {
		return witnessClaim, errors.New("symbol \"eth\" must have null address set as token address")
	}

	// Nonce type casting (*big.Int -> int)
	nonce := int(event.Nonce.Int64())

	// Package the information in a unique EthBridgeClaim
	witnessClaim.EthereumChainID = chainID
	witnessClaim.BridgeContractAddress = bridgeContractAddress
	witnessClaim.Nonce = nonce
	witnessClaim.TokenContractAddress = tokenContractAddress
	witnessClaim.Symbol = symbol
	witnessClaim.EthereumSender = sender
	witnessClaim.ValidatorAddress = valAddr
	witnessClaim.CosmosReceiver = recipient
	witnessClaim.Amount = event.Value.Int64()

	return witnessClaim, nil
}

// ProphecyClaimToSignedOracleClaim packages and signs a prophecy claim's data, returning a new oracle claim
func ProphecyClaimToSignedOracleClaim(event types.ProphecyClaimEvent, key *ecdsa.PrivateKey) (OracleClaim, error) {
	oracleClaim := OracleClaim{}

	// Generate a hashed claim message which contains ProphecyClaim's data
	fmt.Println("Generating unique message for ProphecyClaim", event.ProphecyID)
	message := GenerateClaimMessage(event)

	// Sign the message using the validator's private key
	fmt.Println("Signing message...")
	signature, err := SignClaim(PrefixMsg(message), key)
	if err != nil {
		return oracleClaim, err
	}
	fmt.Println("Signature generated:", hexutil.Encode(signature))

	oracleClaim.ProphecyID = event.ProphecyID
	var message32 [32]byte
	copy(message32[:], message)
	oracleClaim.Message = message32
	oracleClaim.Signature = signature
	return oracleClaim, nil
}

// CosmosMsgToProphecyClaim parses event data from a CosmosMsg, packaging it as a ProphecyClaim
func CosmosMsgToProphecyClaim(event types.CosmosMsg) ProphecyClaim {
	claimType := event.ClaimType
	cosmosSender := event.CosmosSender
	ethereumReceiver := event.EthereumReceiver
	symbol := strings.ToLower(event.Symbol)
	amount := event.Amount

	prophecyClaim := ProphecyClaim{
		ClaimType:        claimType,
		CosmosSender:     cosmosSender,
		EthereumReceiver: ethereumReceiver,
		Symbol:           symbol,
		Amount:           amount,
	}
	return prophecyClaim
}

// BurnLockEventToCosmosMsg parses data from a Burn/Lock event witnessed on Cosmos into a CosmosMsg struct
func BurnLockEventToCosmosMsg(claimType types.Event, attributes []tmKv.Pair) types.CosmosMsg {
	var cosmosSender []byte
	var ethereumReceiver common.Address
	var symbol string
	var amount *big.Int

	for _, attribute := range attributes {
		key := string(attribute.GetKey())
		val := string(attribute.GetValue())

		// Set variable based on the attribute's key
		switch key {
		case ethbridge.AttributeKeyCosmosSender:
			cosmosSender = []byte(val)
		case ethbridge.AttributeKeyEthereumReceiver:
			if !common.IsHexAddress(val) {
				log.Fatal("Invalid recipient address:", val)
			}
			ethereumReceiver = common.HexToAddress(val)
		case ethbridge.AttributeKeyCoins:
			coins, _ := sdk.ParseCoins(val)
			symbol = coins[0].Denom
			amount = coins[0].Amount.BigInt()
		}
	}
	return types.NewCosmosMsg(claimType, cosmosSender, ethereumReceiver, symbol, amount)
}

// isZeroAddress checks an Ethereum address and returns a bool which indicates if it is the null address
func isZeroAddress(address common.Address) bool {
	return address == common.HexToAddress(nullAddress)
}
