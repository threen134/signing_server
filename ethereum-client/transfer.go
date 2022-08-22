package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/secp256k1"
	"github.com/ethereum/go-ethereum/ethclient"
	resty "github.com/go-resty/resty/v2"
	log "github.com/sirupsen/logrus"
)

type Resp struct {
	Uuid      string `json:"uuid"`
	Action    string `json:"action"`
	Signature string `json:"signature"`
}

func main() {
	client, err := ethclient.Dial("https://eth-rinkeby.alchemyapi.io/v2/··")
	if err != nil {
		panic(err)
	}
	pubstring := "0x048b957dd7eca286ff51a0ad4d61148b814d1f40836d757eb594c4303fda7683d84ce9b879a2f8387f7f9f823a011f86f40ef2e3b88bdb5848e81d17a9810cb8ab"
	pubByte, err := hexutil.Decode(pubstring)
	if err != nil {
		panic(err)
	}

	publickey, err := crypto.UnmarshalPubkey(pubByte)
	if err != nil {
		panic(err)
	}

	fromAddress := crypto.PubkeyToAddress(*publickey)
	fmt.Println(fromAddress.Hex()) // 0x96216849c49358B10257cb55b28eA603c874b05E
	nonce, err := client.PendingNonceAt(context.Background(), fromAddress)
	if err != nil {
		panic(err)
	}
	fmt.Println(nonce)

	value := big.NewInt(1000000000000000) // in wei (0.001 eth)
	gasLimit := uint64(21000)             // in units
	gasPrice, err := client.SuggestGasPrice(context.Background())
	if err != nil {
		panic(err)
	}
	toAddress := common.HexToAddress("0xd7c6b20Aa8a7f42cca2a945144426546010eD9C3")
	var data []byte
	tx := types.NewTransaction(nonce, toAddress, value, gasLimit, gasPrice, data)
	chainID, err := client.NetworkID(context.Background())
	if err != nil {
		panic(err)
	}
	log.WithField("chain_id", chainID).Info("chain_id")
	signer := types.NewEIP155Signer(chainID)
	hs := signer.Hash(tx)
	// Create a Resty Client
	restClient := resty.New()
	respbody := &Resp{}
	sig := []byte{}
	_, err = restClient.R().
		EnableTrace().
		SetResult(respbody).
		SetBody(map[string]interface{}{"data": toString(hs.Bytes())}).
		Post("http://localhost:8080:/v1/grep11/key/secp256k1/sign/10b2f9c0-fdc3-402e-a58d-d9931b8313dc")
	if err != nil {
		log.WithError(err).Error("fail to call HPCS")
		panic(err)
	}
	sig = toByte(respbody.Signature)
	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:64])

	sig2 := LocalChange(r, s)
	log.WithField("pub_len", len(pubByte)).WithField("len", len(sig)).WithField("sig", respbody.Signature).Info("sign by HPCS")

	result123 := crypto.VerifySignature(pubByte, hs.Bytes(), sig2)
	log.WithField("result", result123).Info("resut")

	sig = append(sig, 0)

	fmt.Println(len(sig))

	signedTx, err := tx.WithSignature(signer, sig)
	if err != nil {
		log.WithError(err).Error("failed to sign ")
		panic(err)
	}

	err = client.SendTransaction(context.Background(), signedTx)
	if err != nil {
		panic(err)
	}

	fmt.Printf("tx sent: %s \n", signedTx.Hash().Hex())
}

func toString(src []byte) string {
	return base64.RawStdEncoding.EncodeToString(src)
}

func toByte(src string) []byte {
	result, _ := base64.RawStdEncoding.DecodeString(src)
	return result
}

func LocalChange(R, S *big.Int) []byte {
	sig := make([]byte, 65)

	denTwo := big.NewInt(2)
	rightS := big.NewInt(0)
	curve := secp256k1.S256()

	/*
		URL: https://github.com/ethereum/EIPs/blob/master/EIPS/eip-2.md
		All transaction signatures whose s-value is greater than secp256k1n/2 are now considered invalid.
		The ECDSA recover precompiled contract remains unchanged and will keep accepting high s-values; this is useful
		e.g. if a contract recovers old Bitcoin signatures.
	*/
	rightS = rightS.Div(curve.Params().N, denTwo)

	if rightS.Cmp(S) == -1 {
		S = S.Sub(curve.Params().N, S)
		log.Println("S: ", S.String())
		rbytes, sbytes := R.Bytes(), S.Bytes()
		copy(sig[32-len(rbytes):32], rbytes)
		copy(sig[64-len(sbytes):64], sbytes)
		//	log.Println("Result : ", verifyECRecover(hash.Bytes(), sig, expectedPubKey))
	} else {
		rbytes, sbytes := R.Bytes(), S.Bytes()
		copy(sig[32-len(rbytes):32], rbytes)
		copy(sig[64-len(sbytes):64], sbytes)
		//	log.Println("Else Result : ", verifyECRecover(hash.Bytes(), sig, expectedPubKey))
	}
	return sig
}
