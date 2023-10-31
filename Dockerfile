FROM golang:1.21-bookworm

ARG TAMAGO_VERSION
ARG FT_LOG_ORIGIN
ARG FT_LOG_URL
ARG FT_BIN_URL
ARG LOG_PUBLIC_KEY
ARG APPLET_PUBLIC_KEY
ARG OS_PUBLIC_KEY1
ARG OS_PUBLIC_KEY2

# Install dependencies.
RUN apt-get update && apt-get install -y git make wget

RUN wget "https://github.com/usbarmory/tamago-go/releases/download/tamago-go${TAMAGO_VERSION}/tamago-go${TAMAGO_VERSION}.linux-amd64.tar.gz"
RUN tar -xvf "tamago-go${TAMAGO_VERSION}.linux-amd64.tar.gz" -C /

WORKDIR /build

COPY . .

# Set Tamago path for Make rule.
ENV TAMAGO=/usr/local/tamago-go/bin/go

# Firmware transparency parameters for output binary.
ENV FT_LOG_ORIGIN=${FT_LOG_ORIGIN} \
    FT_LOG_URL=${FT_LOG_URL} \
    FT_BIN_URL=${FT_BIN_URL} \
    LOG_PUBLIC_KEY=${LOG_PUBLIC_KEY} \
    APPLET_PUBLIC_KEY=${APPLET_PUBLIC_KEY} \
    OS_PUBLIC_KEY1=${OS_PUBLIC_KEY1} \
    OS_PUBLIC_KEY2=${OS_PUBLIC_KEY2}

RUN make trusted_applet_nosign
