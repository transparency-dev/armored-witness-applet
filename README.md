# ArmoredWitness Applet

This repo contains code for a GoTEE Trusted Applet which implements
a witness. It's intended to be used with the Trusted OS found at
https://github.com/transparency-dev/armored-witness-os.

# Introduction

TODO

# Supported hardware

The following table summarizes currently supported SoCs and boards.

| SoC          | Board                                                                                                                                                                                | SoC package                                                               | Board package                                                                        |
|--------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|---------------------------------------------------------------------------|--------------------------------------------------------------------------------------|
| NXP i.MX6UL  | [USB armory Mk II LAN](https://github.com/usbarmory/usbarmory/wiki)                                                                                                                  | [imx6ul](https://github.com/usbarmory/tamago/tree/master/soc/nxp/imx6ul)  | [usbarmory/mk2](https://github.com/usbarmory/tamago/tree/master/board/usbarmory)      |

The GoTEE [syscall](https://github.com/usbarmory/GoTEE/blob/master/syscall/syscall.go)
interface is implemented for communication between the Trusted OS and Trusted
Applet.

When launched, the witness applet is reachable via SSH through the first
Ethernet port.

```
$ ssh ta@10.0.0.1

date            (time in RFC339 format)?                 # show/change runtime date and time
dns             <fqdn>                                   # resolve domain (requires routing)
exit, quit                                               # close session
hab             <hex SRK hash>                           # secure boot activation (*irreversible*)
help                                                     # this help
led             (white|blue|yellow|green) (on|off)       # LED control
mmc             <hex offset> <size>                      # MMC card read
reboot                                                   # reset device
stack                                                    # stack trace of current goroutine
stackall                                                 # stack trace of all goroutines
status                                                   # status information

>
```

The witness can be also executed under QEMU emulation, including networking
support (requires a `tap0` device routing the Trusted Applet IP address).

> :warning: emulated runs perform partial tests due to lack of full hardware
> support by QEMU.

```
make trusted_applet && make DEBUG=1 trusted_os && make qemu
...
00:00:00 tamago/arm • TEE security monitor (Secure World system/monitor)
00:00:00 SM applet verification
00:00:01 SM applet verified
00:00:01 SM loaded applet addr:0x90000000 entry:0x9007751c size:14228514
00:00:01 SM starting mode:USR sp:0xa0000000 pc:0x9007751c ns:false
00:00:02 tamago/arm • TEE user applet
00:00:02 TA MAC:1a:55:89:a2:69:41 IP:10.0.0.1 GW:10.0.0.2 DNS:8.8.8.8:53
00:00:02 TA requesting SM status
00:00:02 ----------------------------------------------------------- Trusted OS ----
00:00:02 Secure Boot ............: false
00:00:02 Runtime ................: tamago/arm
00:00:02 Link ...................: false
00:00:02 TA starting ssh server (SHA256:eeMIwwN/zw1ov1BvO6sW3wtYi463sq+oLgKhmAew1WE) at 10.0.0.1:22
```

Trusted Applet authentication
=============================

To maintain the chain of trust the OS performes trusted applet authentication
before loading it, to this end the `APPLET_PUBLIC_KEY` and `APPLET_PRIVATE_KEY`
environment variables must be set to the path of either
[signify](https://man.openbsd.org/signify) or
[minisign](https://jedisct1.github.io/minisign/) keys, while compiling.

Example key generation (signify):

```
signify -G -n -p armored-witness-applet.pub -s armored-witness-applet.sec
```

Example key generation (minisign):

```
minisign -G -p armored-witness-applet.pub -s armored-witness-applet.sec
```

Building the compiler
=====================

Build the [TamaGo compiler](https://github.com/usbarmory/tamago-go)
(or use the [latest binary release](https://github.com/usbarmory/tamago-go/releases/latest)):

```
wget https://github.com/usbarmory/tamago-go/archive/refs/tags/latest.zip
unzip latest.zip
cd tamago-go-latest/src && ./all.bash
cd ../bin && export TAMAGO=`pwd`/go
```

Building and executing on ARM targets
=====================================

Build the example trusted applet and kernel executables as follows:

TODO: fix this
```
make trusted_applet && make trusted_os
```

Final executables are created in the `bin` subdirectory, `trusted_os.elf`
should be used for loading through `armored-witness-boot`.

The following targets are available:

| `TARGET`    | Board            | Executing and debugging                                                                                  |
|-------------|------------------|----------------------------------------------------------------------------------------------------------|
| `usbarmory` | UA-MKII-LAN      | [usbarmory/mk2](https://github.com/usbarmory/tamago/tree/master/board/usbarmory)                         |

The targets support native (see relevant documentation links in the table above)
as well as emulated execution (e.g. `make qemu`).

Debugging
---------

An optional Serial over USB console can be used to access Trusted OS and
Trusted Applet logs, it can be enabled when compiling with the `DEBUG`
environment variable set:

```
make trusted_applet && make DEBUG=1 trusted_os
```

The Serial over USB console can be accessed from a Linux host as follows:

```
picocom -b 115200 -eb /dev/ttyACM0 --imap lfcrlf
```

Trusted Applet installation
===========================

TODO

LED status
==========

The [USB armory Mk II](https://github.com/usbarmory/usbarmory/wiki) LEDs
are used, in sequence, as follows:

| Boot sequence                   | Blue | White |
|---------------------------------|------|-------|
| 0. initialization               | off  | off   |
| 1. trusted applet verified      | off  | on    |
| 2. trusted applet execution     | on   | on    |

