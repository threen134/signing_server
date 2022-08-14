
export SIGN_HOST=localhost
export SIGNING_PORT=8080

# 测试连通性
curl ${SIGN_HOST}:${SIGNING_PORT}/v1/grep11/get_mechanismsc

# 产生椭圆曲线Key pair
curl ${SIGN_HOST}:${SIGNING_PORT}/v1/grep11/key/generate_ec_keypair -X POST -s | jq

# 获取公钥
curl ${SIGN_HOST}:${SIGNING_PORT}/v1/grep11/key/public/${KEY_UUID} -s | jq

# 获取被包裹的私钥
curl ${SIGN_HOST}:${SIGNING_PORT}/v1/grep11/key/private/${KEY_UUID} -s | jq

# 使用私钥签名数据
curl ${SIGN_HOST}:${SIGNING_PORT}/v1/grep11/key/sign/${KEY_UUID}  -s -X POST -d '{"data":"the text need to encrypted to verify kay."}'

# 使用公钥验证签名
curl ${SIGN_HOST}:${SIGNING_PORT}/v1/grep11/key/verify/${KEY_UUID}  -s -X POST -d '{"data":"the text need to encrypted to verify kay.","signature":"XqUowVZ2Qlo3CqTD4hWcREqMmWo2PZAhJsGNSKz0bq05qw99HY5U04M1X9266s2DuOc3A2ME/mn87oHfYx4oQw"}'


# 使用master key包裹导入的AES，并持久化到HPDBaaS
curl ${SIGN_HOST}:${SIGNING_PORT}/v1/grep11/key/import_aes -X POST -d '{"key_content":"E5E9FA1BA31ECD1AE84F75CAAA474F3A"}'

#使用导入到AESkey 加密数据与明文AES加密数据结果对比，如果一样就证明导入到key是正确的，并且被master key 包裹了
curl ${SIGN_HOST}:${SIGNING_PORT}/v1/grep11/key/verify_import_aes/${KEY_UUID}  -X POST -d '{"key_content":"E5E9FA1BA31ECD1AE84F75CAAA474F3A","data":"the text need to encrypted to verify kay."}'


# 产生EC 私钥 PEM 格式
openssl ecparam -genkey -name prime256v1 -noout -out ec256-key-pair.pem

# 提取公钥
openssl ec -in ec256-key-pair.pem -pubout > ec256-key-pub.pem

# 上传私钥并持久化
curl ${SIGN_HOST}:${SIGNING_PORT}/v1/grep11/key/import_ec -X POST -s  -F "file=@./ec256-key-pair.pem" | jq

# 签名ec
curl ${SIGN_HOST}:${SIGNING_PORT}/v1/grep11/key/sign/${KEY_UUID} -s -X POST -d '{"data":"the text need to encrypted to verify kay."}' | jq

# 使用公钥验证签名
curl ${SIGN_HOST}:${SIGNING_PORT}/v1/grep11/key/verify/${KEY_UUID} -s -X POST -d '{"data":"the text need to encrypted to verify kay.","signature":"BGXTuJaxu3tdSqiy6vcJOD5t9cGDVpGHuLQmgL/8PejIowclVp3XqtGeRIPPSxcyVXwKp0YiVXF78Ru00BKiWQ"}'

#  签名ec 返回ANS1
curl ${SIGN_HOST}:${SIGNING_PORT}/v1/grep11/key/sign/${KEY_UUID}  -s -X POST -d '{"data":"the text need to encrypted to verify kay.","sig_format":"ans1"}' | jq

# 使用本地公钥验证签名
echo -n "the text need to encrypted to verify kay." > test.data
echo -n "MEUCIBTR7R7lxbjWX1gP+04NcCdPtIeusf3xOg1g0uJUi9eDAiEArfc6Oz5rZSiTpfvuGj6PIETRk+cZeaGnZmt4+ZWAi3Q" |gbase64 --decode -w 0  > signature.sig
openssl pkeyutl -verify -in test.data -sigfile  signature.sig  -pubin  -inkey ec256-key-pub.pem