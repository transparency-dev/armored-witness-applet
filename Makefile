# Copyright 2022 The Armored Witness Applet authors. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

BUILD_USER ?= $(shell whoami)
BUILD_HOST ?= $(shell hostname)
BUILD_DATE ?= $(shell /bin/date -u "+%Y-%m-%d %H:%M:%S")
BUILD_EPOCH := $(shell /bin/date -u "+%s")
BUILD_TAGS = linkramsize,linkramstart,disable_fr_auth,linkprintk,nostatfs
BUILD = ${BUILD_USER}@${BUILD_HOST} on ${BUILD_DATE}
REV = $(shell git rev-parse --short HEAD 2> /dev/null)
DEV_LOG_ORIGIN ?= "DEV.armoredwitness.transparency.dev/${USER}"
GIT_SEMVER_TAG ?= $(shell (git describe --tags --exact-match --match 'v*.*.*' 2>/dev/null || git describe --match 'v*.*.*' --tags 2>/dev/null || git describe --tags 2>/dev/null || echo -n v0.0.${BUILD_EPOCH}+`git rev-parse HEAD`) | tail -c +2 )
FT_BIN_URL ?= "http://$(shell hostname --fqdn):9944/artefacts/"
FT_LOG_URL ?= "http://$(shell hostname --fqdn):9944/log/"
FT_LOG_ORIGIN ?= $(DEV_LOG_ORIGIN)
REST_DISTRIBUTOR_BASE_URL ?= "https://api.transparency.dev"


SHELL = /bin/bash

SIGN = $(shell type -p signify || type -p signify-openbsd || type -p minisign)
SIGN_PWD ?= "armored-witness"

APP := ""
TEXT_START = 0x90010000 # ramStart (defined in mem.go under relevant tamago/soc package) + 0x10000

ifeq ("${BEE}","1")
	TEXT_START := 0x20010000
	BUILD_TAGS := ${BUILD_TAGS},bee
endif

GOENV := GO_EXTLINK_ENABLED=0 CGO_ENABLED=0 GOOS=tamago GOARM=7 GOARCH=arm
ENTRY_POINT := _rt0_arm_tamago

ARCH = "arm"

GOFLAGS = -tags ${BUILD_TAGS} -trimpath \
        -ldflags "-T ${TEXT_START} -E ${ENTRY_POINT} -R 0x1000 \
                  -X 'main.Build=${BUILD}' -X 'main.Revision=${REV}' -X 'main.Version=${GIT_SEMVER_TAG}' \
                  -X 'main.PublicKey=$(shell test ${PUBLIC_KEY} && cat ${PUBLIC_KEY} | tail -n 1)' \
                  -X 'main.GitHubUser=${GITHUB_USER}' -X 'main.GitHubEmail=${GITHUB_EMAIL}' -X 'main.GitHubToken=${GITHUB_TOKEN}' \
                  -X 'main.RestDistributorBaseURL=${REST_DISTRIBUTOR_BASE_URL}' \
                  -X 'main.updateBinariesURL=${FT_BIN_URL}' \
                  -X 'main.updateLogURL=${FT_LOG_URL}' \
                  -X 'main.updateLogOrigin=${FT_LOG_ORIGIN}' \
                  -X 'main.updateLogVerifier=$(shell cat ${LOG_PUBLIC_KEY})' \
                  -X 'main.updateAppletVerifier=$(shell cat ${APPLET_PUBLIC_KEY})' \
                  -X 'main.updateOSVerifier1=$(shell cat ${OS_PUBLIC_KEY1})' \
                  -X 'main.updateOSVerifier2=$(shell cat ${OS_PUBLIC_KEY2})' \
                 "

.PHONY: clean

#### primary targets ####

all: trusted_applet

trusted_applet_nosign: APP=trusted_applet
trusted_applet_nosign: DIR=$(CURDIR)/trusted_applet
trusted_applet_nosign: elf

trusted_applet: APP=trusted_applet
trusted_applet: DIR=$(CURDIR)/trusted_applet
trusted_applet: check_signing_env elf manifest

## Targets for managing a local serverless log instance for dev/testing FT related bits.

## log_initialise initialises the log stored under ${LOG_STORAGE_DIR}.
log_initialise:
	echo "(Re-)initialising log at ${LOG_STORAGE_DIR}"
	go run github.com/transparency-dev/serverless-log/cmd/integrate@a56a93b5681e5dc231882ac9de435c21cb340846 \
		--storage_dir=${LOG_STORAGE_DIR} \
		--origin=${DEV_LOG_ORIGIN} \
		--private_key=${LOG_PRIVATE_KEY} \
		--public_key=${LOG_PUBLIC_KEY} \
		--initialise

## log_applet adds the trusted_applet_manifest file created during the build to the dev FT log.
log_applet: LOG_STORAGE_DIR=$(DEV_LOG_DIR)/log
log_applet: LOG_ARTEFACT_DIR=$(DEV_LOG_DIR)/artefacts
log_applet: ARTEFACT_HASH=$(shell sha256sum ${CURDIR}/bin/trusted_applet.elf | cut -f1 -d" ")
log_applet:
	@if [ "${LOG_PRIVATE_KEY}" == "" -o "${LOG_PUBLIC_KEY}" == "" ]; then \
		@echo "You need to set LOG_PRIVATE_KEY and LOG_PUBLIC_KEY variables"; \
		exit 1; \
	fi
	@if [ "${DEV_LOG_DIR}" == "" ]; then \
		@echo "You need to set the DEV_LOG_DIR variable"; \
		exit 1; \
	fi

	@if [ ! -f ${LOG_STORAGE_DIR}/checkpoint ]; then \
		make log_initialise LOG_STORAGE_DIR="${LOG_STORAGE_DIR}" ; \
	fi
	go run github.com/transparency-dev/serverless-log/cmd/sequence@a56a93b5681e5dc231882ac9de435c21cb340846 \
		--storage_dir=${LOG_STORAGE_DIR} \
		--origin=${DEV_LOG_ORIGIN} \
		--public_key=${LOG_PUBLIC_KEY} \
		--entries=${CURDIR}/bin/trusted_applet_manifest
	-go run github.com/transparency-dev/serverless-log/cmd/integrate@a56a93b5681e5dc231882ac9de435c21cb340846 \
		--storage_dir=${LOG_STORAGE_DIR} \
		--origin=${DEV_LOG_ORIGIN} \
		--private_key=${LOG_PRIVATE_KEY} \
		--public_key=${LOG_PUBLIC_KEY}
	@mkdir -p ${LOG_ARTEFACT_DIR}
	cp ${CURDIR}/bin/trusted_applet.elf ${LOG_ARTEFACT_DIR}/${ARTEFACT_HASH}

#### ARM targets ####

imx: $(APP).imx
elf: $(APP).elf
manifest: $(APP)_manifest

$(APP).bin: CROSS_COMPILE=arm-none-eabi-
$(APP).bin: $(APP).elf
	$(CROSS_COMPILE)objcopy -j .text -j .rodata -j .shstrtab -j .typelink \
	    -j .itablink -j .gopclntab -j .go.buildinfo -j .noptrdata -j .data \
	    -j .bss --set-section-flags .bss=alloc,load,contents \
	    -j .noptrbss --set-section-flags .noptrbss=alloc,load,contents \
	    $(CURDIR)/bin/$(APP).elf -O binary $(CURDIR)/bin/$(APP).bin

$(APP).imx: $(APP).bin $(APP).dcd
	echo "## disabling TZASC bypass in DCD for pre-DDR initialization ##"; \
	chmod 644 $(CURDIR)/bin/$(APP).dcd; \
	echo "DATA 4 0x020e4024 0x00000001  # TZASC_BYPASS" >> $(CURDIR)/bin/$(APP).dcd; \
	mkimage -n $(CURDIR)/bin/$(APP).dcd -T imximage -e $(TEXT_START) -d $(CURDIR)/bin/$(APP).bin $(CURDIR)/bin/$(APP).imx
	# Copy entry point from ELF file
	dd if=$(CURDIR)/bin/$(APP).elf of=$(CURDIR)/bin/$(APP).imx bs=1 count=4 skip=24 seek=4 conv=notrunc

$(APP).dcd: check_tamago
$(APP).dcd: GOMODCACHE=$(shell ${TAMAGO} env GOMODCACHE)
$(APP).dcd: TAMAGO_PKG=$(shell grep "github.com/usbarmory/tamago v" go.mod | awk '{print $$1"@"$$2}')
$(APP).dcd: dcd

#### utilities ####

check_signing_env:
	@if [ "${APPLET_PRIVATE_KEY}" == "" ] || [ ! -f "${APPLET_PRIVATE_KEY}" ]; then \
		echo 'You need to set the APPLET_PRIVATE_KEY variable to a valid signing key path'; \
		exit 1; \
	fi

check_tamago:
	@if [ "${TAMAGO}" == "" ] || [ ! -f "${TAMAGO}" ]; then \
		echo 'You need to set the TAMAGO variable to a compiled version of https://github.com/usbarmory/tamago-go'; \
		exit 1; \
	fi

dcd:
	cp -f $(GOMODCACHE)/$(TAMAGO_PKG)/board/usbarmory/mk2/imximage.cfg $(CURDIR)/bin/$(APP).dcd

clean:
	@rm -fr $(CURDIR)/bin/*

#### application target ####

$(APP).elf: check_tamago
	cd $(DIR) && $(GOENV) $(TAMAGO) build -tags ${BUILD_TAGS} $(GOFLAGS) -o $(CURDIR)/bin/$(APP).elf


$(APP)_manifest: TAMAGO_SEMVER=$(shell ${TAMAGO} version | sed 's/.*go\([0-9]\.[0-9]*\.[0-9]*\).*/\1/')
$(APP)_manifest:
	# Create manifest
	@echo ---------- Manifest --------------
	go run github.com/transparency-dev/armored-witness/cmd/manifest@228f2f6432babe1f1657e150ce0ca4a96ab394da \
		create \
		--git_tag=${GIT_SEMVER_TAG} \
		--git_commit_fingerprint="${REV}" \
		--firmware_file=${CURDIR}/bin/$(APP).elf \
		--firmware_type=TRUSTED_APPLET \
		--tamago_version=${TAMAGO_SEMVER} \
		--private_key_file=${APPLET_PRIVATE_KEY} \
		--output_file=${CURDIR}/bin/trusted_applet_manifest
	@echo ----------------------------------


