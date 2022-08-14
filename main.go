package main

import (
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

func main() {
	log.Info("start signing server...")
	router := gin.Default()

	//get getMechanismInfo
	router.GET("/v1/grep11/get_mechanismsc", getMechanismInfo)

	// generage key pair
	router.POST("/v1/grep11/key/generate_ec_keypair", generageECkeyPair)

	// get public key
	router.GET("/v1/grep11/key/:keyType/:id", findKeyByUUID)

	// sign
	router.POST("/v1/grep11/key/sign/:id", sign)

	// verify signature
	router.POST("/v1/grep11/key/verify/:id", verifySignature)

	// import aes key
	router.POST("/v1/grep11/key/import_aes", importAESKey)

	// import ec key
	router.POST("/v1/grep11/key/import_ec", importECKey)

	// verify imported aes key
	router.POST("/v1/grep11/key/verify_import_aes/:id", verifyImportAESKey)

	router.Run()
}
