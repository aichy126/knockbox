# --platform=$BUILDPLATFORM：多架构构建时在原生架构上编，不进 QEMU。
# CGO_ENABLED=0 让交叉编译零成本，所以 arm64 的镜像也是原生速度出的。
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# 版本号按优先级取：--build-arg VERSION → 构建上下文里的 .version 文件 → dev。
# 留第二条路是因为有的构建系统传不了 build-arg，只能在构建前往工作区写文件。
# 两处 :- 都不能省：ARG 缺省给空串，第二条路才轮得到；.version 缺失或为空，才回落 dev
# —— 否则注入的会是空版本号，比 dev 更难查。
ARG VERSION=
# buildx 自动注入，单架构构建时为空，go 会回落到本机的 GOOS/GOARCH。
ARG TARGETOS
ARG TARGETARCH
# CGO_ENABLED=0 是硬要求：SQLite 用 modernc.org/sqlite 纯 Go 驱动就是为了这个。
RUN VERSION="${VERSION:-$(cat .version 2>/dev/null)}" && \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags "-s -w -X main.Version=${VERSION:-dev}" -o /out/knockbox .

FROM alpine:3.21
# ca-certificates 必须装：要用 TLS 连 api.push.apple.com，缺根证书时全部请求都是
# x509 错误。本机开发不会暴露这个问题，只在容器里出现。
RUN apk add --no-cache ca-certificates tzdata && mkdir -p /app/data
WORKDIR /app
COPY --from=build /out/knockbox /app/knockbox
COPY config.toml.example /app/config.toml
VOLUME ["/app/data"]
EXPOSE 8080
# ENTRYPOINT 只放二进制、子命令放 CMD：
#   docker run <image>                        → serve
#   docker run -it <image> user add admin     → 覆盖 CMD，跑管理命令
# 把 serve 也写进 ENTRYPOINT 的话，后一种用法就没法用了。
ENTRYPOINT ["/app/knockbox"]
CMD ["serve", "-c", "/app/config.toml"]
