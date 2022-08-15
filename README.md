# Signing_server
 
 使用签名服务器用来展示Hyper Protect service 的使用场景

### 环境介绍
![](./img/setup.jpg)
- 通过类似多方合约的方式部署签名服务器到可信执行环境HPVS
- Client 通过RestAPI与签名服务器通信
- 由于签名服务器是以黑盒子的方式部署到HPVS内的，这里log信息通过内网发送到logDNA
- 签名服务器通过GREP11 API 与HPCS 通信，生产环境还需要有MTLS 双向证书验证
- 经过加密的密钥，持久化到HPDBaaS内 
- IAM 对访问做认证以及权限控制


### Client 通过下列endpoint 与签名服务器通信

```sh

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
curl ${SIGN_HOST}:${SIGNING_PORT}/v1/grep11/key/verify/${KEY_UUID}  -s -X POST -d '{"data":"the text need to encrypted to verify kay.","signature":"guoQxLxqOYUbZ2O7jgbLnte4XA0SxSD0xj0/m6SVI0PaIBODQ/WJEZ+By2XqFzrRJyUUc8XFrXcLfHTjFmJjlA"}' |jq


# 使用master key包裹导入的AES，并持久化到HPDBaaS
curl ${SIGN_HOST}:${SIGNING_PORT}/v1/grep11/key/import_aes -X POST -s -d '{"key_content":"E5E9FA1BA31ECD1AE84F75CAAA474F3A"}' |jq

#使用导入的AES Key 加密数据与明文AES加密数据结果对比，如果一样就证明导入到key是正确的，并且被master key 包裹了
curl ${SIGN_HOST}:${SIGNING_PORT}/v1/grep11/key/verify_import_aes/${KEY_UUID}  -X POST -d '{"key_content":"E5E9FA1BA31ECD1AE84F75CAAA474F3A","data":"the text need to encrypted to verify kay."}'

####################### 导入外部密钥
# 产生ECDSA 私钥 PEM 格式的密钥，作为外部的密钥
openssl ecparam -genkey -name prime256v1 -noout -out ec256-key-pair.pem

# 提取公钥
openssl ec -in ec256-key-pair.pem -pubout > ec256-key-pub.pem

# 上传外部的私钥并持久化到HPDBaaS
curl ${SIGN_HOST}:${SIGNING_PORT}/v1/grep11/key/import_ec -X POST -s  -F "file=@./ec256-key-pair.pem" | jq

# 使用导入到私钥签名 
curl ${SIGN_HOST}:${SIGNING_PORT}/v1/grep11/key/sign/${KEY_UUID} -s -X POST -d '{"data":"the text need to encrypted to verify kay."}' | jq

# 使用公钥验证签名
curl ${SIGN_HOST}:${SIGNING_PORT}/v1/grep11/key/verify/${KEY_UUID} -s -X POST -d '{"data":"the text need to encrypted to verify kay.","signature":"UjVXX0/CcX4RzZmMW5lkcK5/N8oUSCKd5eFiHUDNgsby74skSD3cbJN4bTesinVq1b4QzbLSnDpuNkl89i8FSw"}'

#  签名ec 返回ANS1的签名
curl ${SIGN_HOST}:${SIGNING_PORT}/v1/grep11/key/sign/${KEY_UUID}  -s -X POST -d '{"data":"the text need to encrypted to verify kay.","sig_format":"ans1"}' | jq

# 使用本地公钥验证签名
# 通过HPCS 产生签名，并使用导入之前到公钥验证签名，签名验证正确，证明被导入到私钥是正确的，且被双层加密
echo -n "the text need to encrypted to verify kay." > test.data
echo -n "MEYCIQDd7kmZc0E/zPq7vDkQ/VmeM0OaVS1XnGsdi/e10xYUBwIhAOEcYNXoAYAeIcOkUlrDr4g8MjQVLZKa8q4aQQNps5IJ" |gbase64 --decode -w 0  > signature.sig
openssl pkeyutl -verify -in test.data -sigfile  signature.sig  -pubin  -inkey ec256-key-pub.pem


```
# GREP11 API 使用举例时序图
![](./img/GREP11%20API%20%20使用场景说明-详细版本.jpg)