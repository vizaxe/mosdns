#!/bin/bash
CGO_ENABLED=0 go build -o mosdns -trimpath -ldflags=-s -ldflags=-w "-ldflags=-X=main.version=$(git describe --tags --long --always)"