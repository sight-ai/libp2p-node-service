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
// Windows
GOOS=windows GOARCH=amd64 go build -o ./dist/sight-libp2p-node-amd64.exe

```