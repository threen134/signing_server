package main

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/IBM-Cloud/hpcs-grep11-go/ep11"
	pb "github.com/IBM-Cloud/hpcs-grep11-go/grpc"
	"github.com/IBM-Cloud/hpcs-grep11-go/util"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

type SignBody struct {
	Data string
}

type ImportKeyBody struct {
	Name string `json:"key_name"`
	Type string `json:"key_type"`
	Key  string `json:"key_content"`
}
type VerifyBody struct {
	KeyId     string `json:"key_uuid"`
	Data      string `json:"data"`
	Signature string `json:"signature"`
}

type VerifyImportAESKeyBody struct {
	KeyId string `json:"key_uuid"`
	Data  string `json:"data"`
	Key   string `json:"key_content"`
}

func (v *VerifyBody) String() string {
	ks, _ := json.Marshal(v)
	return string(ks)
}

// generate EC key pair
func generageECkeyPair(ctx *gin.Context) {
	log.Info("start generte EC key")
	aes, err := loadAesKEK("./secureEnclave/kek.key")
	if err != nil {
		ctx.AbortWithError(500, err)
	}

	publicKey, privateKey, err := generateECKeyPair()
	if err != nil {
		ctx.AbortWithError(500, err)
	}

	encryptedPrivateKey, err := encryptAES(aes, privateKey)
	if err != nil {
		ctx.AbortWithError(500, err)
	}
	encryptedPrivateKeyStr := toString(encryptedPrivateKey)
	pubKeyStr := toString(publicKey)
	log.WithField("private_encrypt", encryptedPrivateKeyStr).WithField("public", pubKeyStr).Info("generate ec key pair")

	keys, err := insertKey(
		getGlobal().db,
		encryptedPrivateKeyStr,
		pubKeyStr,
	)
	if err != nil {
		ctx.AbortWithError(500, err)
	}
	ctx.JSON(http.StatusOK, gin.H{
		"uuid":    keys.Uuid,
		"public":  encryptedPrivateKeyStr,
		"private": pubKeyStr,
	})

	ctx.String(http.StatusOK, keys.Uuid)
}

// sign by private key
func importAESKey(ctx *gin.Context) {
	requestBody := ImportKeyBody{}
	if err := ctx.BindJSON(&requestBody); err != nil {
		log.WithError(err).Error("fail to read json body")
		ctx.AbortWithError(400, err)
	}
	tempAESKey, err := generateAESKey()
	if err != nil {
		log.WithError(err).Error("failed to generate temp AES key")
		ctx.AbortWithError(500, err)
	}

	iv, err := generateIV()
	if err != nil {
		log.WithError(err).Error("failed to generate iv")
		ctx.AbortWithError(500, err)
	}
	importeRawKey := requestBody.Key
	encryptedImportedKey, err := encryptAESCBC(tempAESKey, toByte(importeRawKey), iv)
	if err != nil {
		log.WithError(err).Error("failed to encrypted imported AES key")
		ctx.AbortWithError(500, err)
	}
	importkey, err := unwrapByAESCBC(encryptedImportedKey, tempAESKey, iv)
	if err != nil {
		log.WithError(err).Error("failed to unwrap AES key")
		ctx.AbortWithError(500, err)
	}
	importkeyStr := toString(importkey)
	log.WithField("importkey", importkeyStr).Info("unwrap key success")
	keys, err := insertKey(
		getGlobal().db,
		importkeyStr,
		"",
	)
	if err != nil {
		log.WithError(err).Error("failed to insert AES key")
		ctx.AbortWithError(500, err)
	}
	ctx.JSON(http.StatusOK, gin.H{
		"uuid":    keys.Uuid,
		"private": keys.PrivateKey,
	})
}

func verifyImportAESKey(ctx *gin.Context) {
	requestBody := VerifyImportAESKeyBody{}
	keyUUID := ctx.Param("id")
	if err := ctx.BindJSON(&requestBody); err != nil {
		log.WithError(err).Error("fail to read json body")
		ctx.AbortWithError(400, err)
	}
	iv, err := generateIV()
	if err != nil {
		log.WithError(err).Error("failed to generate iv")
		ctx.AbortWithError(500, err)
	}
	log.WithFields(
		log.Fields{
			"key_uuid":    keyUUID,
			"data":        requestBody.Data,
			"key_content": requestBody.Key,
		}).Info("load body")

	target1, err := AesEncryptLocal([]byte(requestBody.Data), toByte(requestBody.Key), iv)
	if err != nil {
		log.WithError(err).Error("failed to encrypted byte by local")
		ctx.AbortWithError(500, err)
	}
	keystore := getKeyByUUID(getGlobal().db, keyUUID)
	log.WithField("key", keystore).Info("load key success")
	target2, err := encryptAESCBC(toByte(keystore.PrivateKey), []byte(requestBody.Data), iv)
	if err != nil {
		log.WithError(err).Error("failed to encrypted byte by hpcs")
		ctx.AbortWithError(500, err)
	}

	result := false
	if bytes.Equal(target1, target2) {
		result = true
	}
	ctx.JSON(http.StatusOK, gin.H{"result": result})
}

// sign by private key
func sign(ctx *gin.Context) {
	aes, err := loadAesKEK("./secureEnclave/kek.key")
	if err != nil {
		ctx.AbortWithError(500, err)
	}
	requestBody := SignBody{}

	if err := ctx.BindJSON(&requestBody); err != nil {
		log.WithError(err).Error("fail to read json body")
		ctx.AbortWithError(400, err)
	}

	keyUUID := ctx.Param("id")
	keystore := getKeyByUUID(getGlobal().db, keyUUID)
	log.WithField("key_uuid", keyUUID).WithField("data", requestBody.Data).Info("start sign")
	rawPrivate := toByte(keystore.PrivateKey)
	privatekey, err := decryptAES(aes, rawPrivate)
	if err != nil {
		log.WithError(err).Error("failed to decrypt private key")
		ctx.AbortWithError(500, err)
	}
	data := bytes.NewBufferString(requestBody.Data).Bytes()
	sig, err := signEC(privatekey, data)
	if err != nil {
		log.WithError(err).Error("failed to sign data")
		ctx.AbortWithError(500, err)
	}
	ctx.JSON(http.StatusOK, gin.H{
		"uuid":      keystore.Uuid,
		"action":    "sign",
		"signature": toString(sig),
	})
}

func findKeyByUUID(ctx *gin.Context) {
	keyType := ctx.Param("keyType")
	keyUUID := ctx.Param("id")
	if keyUUID == "" {
		ctx.AbortWithError(400, fmt.Errorf("invalid key id"))
	}

	key := getKeyByUUID(getGlobal().db, keyUUID)
	if key == nil {
		ctx.AbortWithError(400, fmt.Errorf("invalid key id"))
	}
	if keyType == "public" {
		ctx.JSON(http.StatusOK, gin.H{
			"uuid":    key.Uuid,
			"type":    "public",
			"content": key.PublicKey,
		})
		return
	}
	if keyType == "private" {
		ctx.JSON(http.StatusOK, gin.H{
			"uuid":    key.Uuid,
			"type":    "private",
			"content": key.PrivateKey,
		})
		return
	}
	ctx.AbortWithError(400, fmt.Errorf("invalid key type"))
}

func verifySignature(ctx *gin.Context) {
	requestBody := VerifyBody{}
	if err := ctx.BindJSON(&requestBody); err != nil {
		log.WithError(err).Error("fail to read json body")
		ctx.AbortWithError(400, err)
	}

	log.WithField("requestBody", requestBody).Info("start sign")
	keyUUID := ctx.Param("id")
	keystore := getKeyByUUID(getGlobal().db, keyUUID)
	data := bytes.NewBufferString(requestBody.Data).Bytes()
	result, err := verifyEC(toByte(requestBody.Signature), toByte(keystore.PublicKey), data)
	if err != nil {
		log.WithError(err).Error("failed to verify signature")
		ctx.AbortWithError(500, err)
	}
	ctx.JSON(http.StatusOK, gin.H{"result": result})
}

func getMechanismInfo(ctx *gin.Context) {
	mc, err := listMechanismInfo()
	if err != nil {
		ctx.AbortWithError(500, err)
	}
	ctx.String(http.StatusOK, mc)
}

func loadAesKEK(kekPath string) ([]byte, error) {
	if len(kek) != 0 {
		return kek, nil
	}
	log.WithField("kay_path", kekPath).Info("get kek")
	kek, err := ioutil.ReadFile(kekPath)
	if err == nil && len(kek) > 0 {
		log.Info("load kek from local file")
		return kek, nil
	}
	log.WithError(err).Error("failed to read key path from load, start to generate a new kek")
	log.Info("generate a new KEK")
	kek, err = generateAESKey()
	if err != nil {
		log.WithError(err).Error("failed to generate KEK")
		return nil, err
	}
	log.WithField("kek", toString(kek)).Info("success generate KEK")
	log.Info("write kek to secure enclave data volume")
	if err = ioutil.WriteFile(kekPath, kek, 0644); err != nil {
		log.WithError(err).Error("generate key fail")
		return nil, err
	}
	return kek, nil
}

func generateIV() ([]byte, error) {
	conn, err := getGlobal().grpcClient()
	if err != nil {
		return nil, fmt.Errorf("could not connect to server: %s", err)
	}
	defer conn.Close()

	cryptoClient := pb.NewCryptoClient(conn)
	rngTemplate := &pb.GenerateRandomRequest{
		Len: (uint64)(ep11.AES_BLOCK_SIZE),
	}
	// Generate a 16 byte initialization vector for the encrypt/decrypt operations
	rng, err := cryptoClient.GenerateRandom(context.Background(), rngTemplate)
	if err != nil {
		return nil, err
	}
	iv := rng.Rnd[:ep11.AES_BLOCK_SIZE]
	return iv, nil
}

// generate aes to kek
func generateAESKey() ([]byte, error) {
	conn, err := getGlobal().grpcClient()
	if err != nil {
		return nil, fmt.Errorf("could not connect to server: %s", err)
	}
	defer conn.Close()

	cryptoClient := pb.NewCryptoClient(conn)
	keyLen := 128 // bits

	// Setup the AES key's attributes
	keyTemplate := ep11.EP11Attributes{
		ep11.CKA_VALUE_LEN:   keyLen / 8,
		ep11.CKA_WRAP:        true,
		ep11.CKA_UNWRAP:      true,
		ep11.CKA_ENCRYPT:     true,
		ep11.CKA_DECRYPT:     true,
		ep11.CKA_EXTRACTABLE: false, // set to false!
	}

	generateKeyRequest := &pb.GenerateKeyRequest{
		Mech:     &pb.Mechanism{Mechanism: ep11.CKM_AES_KEY_GEN},
		Template: util.AttributeMap(keyTemplate),
	}

	generateKeyResponse, err := cryptoClient.GenerateKey(context.Background(), generateKeyRequest)
	if err != nil {
		log.WithError(err).Error("generate kek failed")
		return nil, err
	}
	log.Info("generate kek success")
	return generateKeyResponse.GetKeyBytes(), nil
}

// 通过KEK加密私钥
func encryptAES(kek, plain []byte) ([]byte, error) {
	conn, err := getGlobal().grpcClient()
	if err != nil {
		return nil, fmt.Errorf("could not connect to server: %s", err)
	}
	defer conn.Close()

	cryptoClient := pb.NewCryptoClient(conn)

	encryptRequest := &pb.EncryptSingleRequest{
		Mech:  &pb.Mechanism{Mechanism: ep11.CKM_AES_ECB},
		Key:   kek,
		Plain: plain,
	}

	encryptResponse, err := cryptoClient.EncryptSingle(context.Background(), encryptRequest)
	if err != nil {
		log.WithError(err).Error("failed to encrypt key")
		return nil, err
	}
	fmt.Println("通过KEK加密私钥成功")
	return encryptResponse.GetCiphered(), nil
}

func encryptAESCBC(key, plain, iv []byte) ([]byte, error) {
	conn, err := getGlobal().grpcClient()
	if err != nil {
		return nil, fmt.Errorf("could not connect to server: %s", err)
	}
	defer conn.Close()

	cryptoClient := pb.NewCryptoClient(conn)

	encryptRequest := &pb.EncryptSingleRequest{
		Mech:  &pb.Mechanism{Mechanism: ep11.CKM_AES_CBC_PAD, Parameter: util.SetMechParm(iv)},
		Key:   key,
		Plain: plain,
	}

	encryptResponse, err := cryptoClient.EncryptSingle(context.Background(), encryptRequest)
	if err != nil {
		log.WithError(err).Error("failed to encrypt key by encryptAESCBC")
		return nil, err
	}
	log.Info("通过aes cbc pad 加密私钥成功")
	return encryptResponse.GetCiphered(), nil
}

// 通过KEK解密私钥
func decryptAES(kek, ciphered []byte) ([]byte, error) {
	conn, err := getGlobal().grpcClient()
	if err != nil {
		return nil, fmt.Errorf("could not connect to server: %s", err)
	}
	defer conn.Close()

	cryptoClient := pb.NewCryptoClient(conn)

	decryptSingleRequest := &pb.DecryptSingleRequest{
		Mech:     &pb.Mechanism{Mechanism: ep11.CKM_AES_ECB},
		Key:      kek,
		Ciphered: ciphered,
	}

	decryptResponse, err := cryptoClient.DecryptSingle(context.Background(), decryptSingleRequest)
	if err != nil {
		log.WithError(err).Error("fail to decrypt private key by KEK")
		return nil, err
	}
	fmt.Println("success to decrypt private key by KEK")
	return decryptResponse.GetPlain(), nil
}

func unwrapByAESCBC(encryptedPrivateKey, aesKey, iv []byte) ([]byte, error) {
	keyLen := 128 // bits

	conn, err := getGlobal().grpcClient()
	if err != nil {
		return nil, fmt.Errorf("could not connect to server: %s", err)
	}
	defer conn.Close()

	cryptoClient := pb.NewCryptoClient(conn)

	// Setup the AES key's attributes
	aesUnwrapKeyTemplate := ep11.EP11Attributes{
		ep11.CKA_CLASS:       ep11.CKO_SECRET_KEY,
		ep11.CKA_KEY_TYPE:    ep11.CKK_AES,
		ep11.CKA_VALUE_LEN:   keyLen / 8,
		ep11.CKA_WRAP:        false,
		ep11.CKA_UNWRAP:      false,
		ep11.CKA_ENCRYPT:     true,
		ep11.CKA_DECRYPT:     true,
		ep11.CKA_SENSITIVE:   true,
		ep11.CKA_EXTRACTABLE: false, // set to false!
	}

	unwrapRequest := &pb.UnwrapKeyRequest{
		Mech:     &pb.Mechanism{Mechanism: ep11.CKM_AES_CBC_PAD, Parameter: util.SetMechParm(iv)},
		KeK:      aesKey,
		Wrapped:  encryptedPrivateKey,
		Template: util.AttributeMap(aesUnwrapKeyTemplate),
	}

	// Unwrap the AES key
	unwrappedResponse, err := cryptoClient.UnwrapKey(context.Background(), unwrapRequest)
	if err != nil {
		log.WithError(err).Error("failed to unwrapByAESCBC ")
	}
	return unwrappedResponse.GetUnwrappedBytes(), nil
}

// generate EC Key pair
func generateECKeyPair() (public, private []byte, err error) {
	conn, err := getGlobal().grpcClient()
	if err != nil {
		return nil, nil, fmt.Errorf("could not connect to server: %s", err)
	}
	defer conn.Close()

	cryptoClient := pb.NewCryptoClient(conn)
	ecParameters, err := asn1.Marshal(util.OIDNamedCurveED25519)
	if err != nil {
		log.WithError(err).Error("unable to encode parameter OID")
		return nil, nil, err
	}

	publicKeyTemplate := ep11.EP11Attributes{
		ep11.CKA_EC_PARAMS:   ecParameters,
		ep11.CKA_VERIFY:      true,
		ep11.CKA_EXTRACTABLE: false,
	}
	privateKeyTemplate := ep11.EP11Attributes{
		ep11.CKA_SIGN:        true,
		ep11.CKA_EXTRACTABLE: false,
	}
	generateKeyPairRequest := &pb.GenerateKeyPairRequest{
		Mech:            &pb.Mechanism{Mechanism: ep11.CKM_EC_KEY_PAIR_GEN},
		PubKeyTemplate:  util.AttributeMap(publicKeyTemplate),
		PrivKeyTemplate: util.AttributeMap(privateKeyTemplate),
	}
	generateKeyPairResponse, err := cryptoClient.GenerateKeyPair(context.Background(), generateKeyPairRequest)
	if err != nil {
		log.WithError(err).Error("generateECKeyPair error")
		return nil, nil, err
	}
	fmt.Println("success generate EC key pair")
	return generateKeyPairResponse.GetPubKeyBytes(), generateKeyPairResponse.GetPrivKeyBytes(), nil
}

func signEC(privateKey, data []byte) (signature []byte, err error) {
	log.Info("us ec to sign data")
	conn, err := getGlobal().grpcClient()
	if err != nil {
		return nil, fmt.Errorf("could not connect to server: %s", err)
	}
	defer conn.Close()

	cryptoClient := pb.NewCryptoClient(conn)

	signRequest := &pb.SignSingleRequest{
		Mech:    &pb.Mechanism{Mechanism: ep11.CKM_IBM_ED25519_SHA512},
		PrivKey: privateKey,
		Data:    data,
	}

	signSingleResponse, err := cryptoClient.SignSingle(context.Background(), signRequest)
	if err != nil {
		log.WithError(err).Error("fail to sign data")
		return nil, err
	}

	fmt.Println("sign success")
	return signSingleResponse.GetSignature(), nil
}

func verifyEC(signature, pubKey, data []byte) (bool, error) {
	log.Info("使用椭圆曲线算法公钥验证签名")
	conn, err := getGlobal().grpcClient()
	if err != nil {
		return false, fmt.Errorf("could not connect to server: %s", err)
	}
	defer conn.Close()
	cryptoClient := pb.NewCryptoClient(conn)

	verifySingleRequest := &pb.VerifySingleRequest{
		Mech:      &pb.Mechanism{Mechanism: ep11.CKM_IBM_ED25519_SHA512},
		PubKey:    pubKey,
		Data:      data,
		Signature: signature,
	}

	_, err = cryptoClient.VerifySingle(context.Background(), verifySingleRequest)
	if ok, ep11Status := util.Convert(err); !ok {
		if ep11Status.Code == ep11.CKR_SIGNATURE_INVALID {
			log.WithError(err).Info("invalid signature")
			return false, nil
		}
		log.WithField("ep11Status.Code", ep11Status.Code).WithField("ep11Status.Detail", ep11Status.Detail).Error("verify err")
		return false, fmt.Errorf("verify error: [%d]: %s", ep11Status.Code, ep11Status.Detail)
	}
	log.Info("验签成功")
	return true, nil
}

// https://blog.csdn.net/qq_28058509/article/details/120997693
// https://www.systutorials.com/how-to-generate-rsa-private-and-public-key-pair-in-go-lang/
func generateRSAPairLocal() (privateKeyBytes, publicKeyBytes []byte) {
	//生成私钥
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		panic(err)
	}
	//生成公钥
	publicKey := &privateKey.PublicKey
	privateKeyBytes = x509.MarshalPKCS1PrivateKey(privateKey)
	publicKeyBytes = x509.MarshalPKCS1PublicKey(publicKey)
	return privateKeyBytes, publicKeyBytes
}

func listMechanismInfo() (string, error) {
	log.Info("get mechanism")
	conn, err := getGlobal().grpcClient()
	if err != nil {
		return "", fmt.Errorf("could not connect to server: %s", err)
	}
	defer conn.Close()

	cryptoClient := pb.NewCryptoClient(conn)

	mechanismListRequest := &pb.GetMechanismListRequest{}

	// Retrieve a list of all supported mechanisms
	mechanismListResponse, err := cryptoClient.GetMechanismList(context.Background(), mechanismListRequest)
	if err != nil {
		return "", fmt.Errorf("Get mechanism list error: %s", err)
	}
	fmt.Printf("Got mechanism list:\n%v ...\n", mechanismListResponse.Mechs[:1])

	mechanismInfoRequest := &pb.GetMechanismInfoRequest{
		Mech: ep11.CKM_RSA_PKCS,
	}

	// Retrieve information about the CKM_RSA_PKCS mechanism
	mechanismInfoResponse, err := cryptoClient.GetMechanismInfo(context.Background(), mechanismInfoRequest)
	if err != nil {
		panic(fmt.Errorf("Get mechanism info error: %s", err))
	}

	return mechanismInfoResponse.GetMechInfo().String(), nil
}

func chunkSlice(slice []byte, chunkSize int) [][]byte {
	var chunks [][]byte
	for i := 0; i < len(slice); i += chunkSize {
		end := i + chunkSize

		// necessary check to avoid slicing beyond
		// slice capacity
		if end > len(slice) {
			end = len(slice)
		}

		chunks = append(chunks, slice[i:end])
	}

	return chunks
}

//AesEncrypt 加密
func AesEncryptLocal(data []byte, key []byte, iv []byte) ([]byte, error) {
	//创建加密实例
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	//判断加密快的大小
	blockSize := block.BlockSize()
	//填充
	encryptBytes := pkcs7Padding(data, blockSize)
	//初始化加密数据接收切片
	crypted := make([]byte, len(encryptBytes))
	//使用cbc加密模式
	blockMode := cipher.NewCBCEncrypter(block, iv)
	//执行加密
	blockMode.CryptBlocks(crypted, encryptBytes)
	return crypted, nil
}

//pkcs7Padding 填充
func pkcs7Padding(data []byte, blockSize int) []byte {
	//判断缺少几位长度。最少1，最多 blockSize
	padding := blockSize - len(data)%blockSize
	//补足位数。把切片[]byte{byte(padding)}复制padding个
	padText := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(data, padText...)
}

func toString(src []byte) string {
	return base64.RawStdEncoding.EncodeToString(src)
}

func toByte(src string) []byte {
	result, _ := base64.RawStdEncoding.DecodeString(src)
	return result
}