#!/usr/bin/env bash
set -e

version="$1"

# Directory where the binaries were placed. 
binDir="$2"

function package {
  os=$1
  arch=$2
 	
	echo Packaging ${os}...
 	folder=$binDir/ScPrime-$version-$os-$arch
 	(
		cd $binDir
		zip -rq ScPrime-$version-$os-$arch.zip ScPrime-$version-$os-$arch
		sha256sum  ScPrime-$version-$os-$arch.zip >> ScPrime-$version-SHA256SUMS.txt
 	)
}

# Package amd64 binaries.
for os in darwin linux windows; do
  package "$os" "amd64"
done

# Package Raspberry Pi binaries.
package "linux" "arm64"
