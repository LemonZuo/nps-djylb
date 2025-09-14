#!/bin/bash

export GOPROXY=direct

# Create build directory
mkdir -p build

sudo apt-get update
sudo apt-get install gcc-mingw-w64-i686 gcc-multilib

# Build SDK
env GOOS=windows GOARCH=386 CGO_ENABLED=1 CC=i686-w64-mingw32-gcc go build -tags sdk -ldflags "-s -w -extldflags -static -extldflags -static" -buildmode=c-shared -o build/npc_sdk.dll cmd/npc/sdk.go
env GOOS=linux GOARCH=386 CGO_ENABLED=1 CC=gcc go build -tags sdk -ldflags "-s -w -extldflags -static -extldflags -static" -buildmode=c-shared -o build/npc_sdk.so cmd/npc/sdk.go
cp npc_sdk.h build/
cd build && tar -czvf npc_sdk_old.tar.gz npc_sdk.dll npc_sdk.so npc_sdk.h && cd ..

# Build Windows 386 client
CGO_ENABLED=0 GOOS=windows GOARCH=386 go build -ldflags "-s -w -extldflags -static -extldflags -static -X 'github.com/djylb/nps/lib/install.BuildTarget=win7'" -o build/npc.exe ./cmd/npc/npc.go
cd build && tar -czvf windows_386_client_old.tar.gz npc.exe ../conf/npc.conf ../conf/multi_account.conf && cd ..

# Build Windows amd64 client
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags "-s -w -extldflags -static -extldflags -static -X 'github.com/djylb/nps/lib/install.BuildTarget=win7'" -o build/npc.exe ./cmd/npc/npc.go
cd build && tar -czvf windows_amd64_client_old.tar.gz npc.exe ../conf/npc.conf ../conf/multi_account.conf && cd ..

# Build Windows amd64 server
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags "-s -w -extldflags -static -extldflags -static -X 'github.com/djylb/nps/lib/install.BuildTarget=win7'" -o build/nps.exe ./cmd/nps/nps.go
cd build && tar -czvf windows_amd64_server_old.tar.gz ../conf/nps.conf ../web/views ../web/static nps.exe && cd ..

# Build Windows 386 server
CGO_ENABLED=0 GOOS=windows GOARCH=386 go build -ldflags "-s -w -extldflags -static -extldflags -static -X 'github.com/djylb/nps/lib/install.BuildTarget=win7'" -o build/nps.exe ./cmd/nps/nps.go
cd build && tar -czvf windows_386_server_old.tar.gz ../conf/nps.conf ../web/views ../web/static nps.exe && cd ..

echo "All build artifacts are in the build/ directory"