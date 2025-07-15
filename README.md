# Libp2p Go
## install dependencies
```
go mod tidy

cp .env.template .env
```

## build
```
go build
```


## start
```
go run .
```

## package
```
// Windows x86_64
GOOS=windows GOARCH=amd64 go build -o ./dist/sight-libp2p-node-win-x86_64.exe

// Linux x86_64
GOOS=linux GOARCH=amd64 go build -o ./dist/sight-libp2p-node-linux-x86_64

// MacOs arm64
GOOS=darwin GOARCH=arm64 go build -o ./dist/sight-libp2p-node-macos-arm64

// MacOS x86_64
GOOS=darwin GOARCH=amd64 go build -o ./dist/sight-libp2p-node-macos-x86_64

```

## run package (assume the package is called sight-libp2p-node)
```
// 查看帮助
./dist/sight-libp2p-node --help

// 使用默认配置（普通环境）
./dist/sight-libp2p-node

// 修改配置样例参考
./dist/sight-libp2p-node --bootstrap-addrs /ip4/xxx --node-port 25050 --libp2p-port 5010 --api-port 9716 --is-gateway 1

// 切换至benchmark(仅需修改IP) 
./dist/sight-libp2p-node --bootstrap-addrs /ip4/34.84.180.62/tcp/15001/p2p/12D3KooWPjceQrSwdWXPyLLeABRXmuqt69Rg3sBYbU1Nft9HyQ6X,/ip4/34.84.180.62/tcp/15002/p2p/12D3KooWH3uVF6wv47WnArKHk5p6cvgCJEb74UTmxztmQDc298L3,/ip4/34.84.180.62/tcp/15003/p2p/12D3KooWQYhTNQdmr3ArTeUHRYzFg94BKyTkoWBDWez9kSCVe2Xo,/ip4/34.84.180.62/tcp/15004/p2p/12D3KooWLJtG8fd2hkQzTn96MrLvThmnNQjTUFZwGEsLRz5EmSzc,/ip4/34.84.180.62/tcp/15005/p2p/12D3KooWSHj3RRbBjD15g6wekV8y3mm57Pobmps2g2WJm6F67Lay,/ip4/34.84.180.62/tcp/15006/p2p/12D3KooWDMCQbZZvLgHiHntG1KwcHoqHPAxL37KvhgibWqFtpqUY,/ip4/34.84.180.62/tcp/15007/p2p/12D3KooWLnZUpcaBwbz9uD1XsyyHnbXUrJRmxnsMiRnuCmvPix67,/ip4/34.84.180.62/tcp/15008/p2p/12D3KooWQ8vrERR8bnPByEjjtqV6hTWehaf8TmK7qR1cUsyrPpfZ

```

## run local p2p environment
```
// change the BOOTSTRAP_ADDRS in .env to localhost (34.146.228.26 -> 127.0.0.1)

// start bootstrap nodes
go run ./bootstrap/main.go

// start client libp2p node
go run . 

// start gateway libp2p node
NODE_PORT=15052 LIBP2P_REST_API=4012 API_PORT=8718 IS_GATEWAY=1 go run .
```

## Libp2p REST API
```
// Send message
curl -X POST -H "Content-Type: application/json" -d '{"to": "peerId", "payload": {"key": "value"}}' http://localhost:4010/libp2p/send

// Find peer
curl http://localhost:{port}/libp2p/find-peer/{peerId}

// Get public key
curl http://localhost:{port}/libp2p/public-key/{peerId}

// Health check
curl http://localhost:{port}/health
```