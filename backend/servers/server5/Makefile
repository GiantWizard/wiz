# Makefile for building the real MEGA exporter

MEGA_SDK_INCLUDE = /path/to/mega/sdk/include
MEGA_SDK_LIB = /path/to/mega/sdk/lib

exporter: exporter.cpp
	clang++ -Wall -Wextra -isysroot $(shell xcrun --sdk macosx --show-sdk-path) exporter.cpp -o exporter \
	    -I$(MEGA_SDK_INCLUDE) \
	    -L$(MEGA_SDK_LIB) -lmegaclient \
	    -lcurl -lcrypto++ -lfreeimage

clean:
	rm -f exporter
