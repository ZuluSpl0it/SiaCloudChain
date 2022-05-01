#!/usr/bin/env bash
set -e

version="$1"

# Directory where the binaries produces by build-release.sh are stored.
binDir="$2"

# Directory of the ScPrime-UI Repo.
uiDir="$3"

if [ -z "$version" ] || [ -z "$binDir" ] || [ -z "$uiDir" ]; then
  echo "Usage: $0 VERSION BIN_DIRECTORY UI_DIRECTORY"
  exit 1
fi

echo Version: "${version}"
echo Binaries Directory: "${binDir}"
echo UI Directory: "${uiDir}"
echo ""

if [ "$SIA_SILENT_RELEASE" != "true" ]; then
	read -p "Continue (y/n)?" CONT
	if [ "$CONT" != "y" ]; then
		exit 1
	fi
fi
echo "Building SiaCloud-UI...";

# Get the absolute paths to avoid funny business with relative paths.
uiDir=$(realpath "${uiDir}")
binDir=$(realpath "${binDir}")

# Remove previously built UI binaries, if the release folder exists
	rm -r "${uiDir}"/release/ || true;

cd "${uiDir}"

# Copy over all the spc/spd binaries.
mkdir -p bin/{linux,mac,win}
cp "${binDir}"/ScPrime-"${version}"-darwin-amd64/spc bin/mac/
cp "${binDir}"/ScPrime-"${version}"-darwin-amd64/spd bin/mac/

cp "${binDir}"/ScPrime-"${version}"-linux-amd64/spc bin/linux/
cp "${binDir}"/ScPrime-"${version}"-linux-amd64/spd bin/linux/

cp "${binDir}"/ScPrime-"${version}"-windows-amd64/spc.exe bin/win/
cp "${binDir}"/ScPrime-"${version}"-windows-amd64/spd.exe bin/win/

# Build yarn deps.
#yarn
yarn --ignore-engines

# Build each of the UI binaries.
yarn package-linux 
yarn package-win
yarn package

# Copy the UI binaries into the binDir. Also change the name at the same time.
# The UI builder doesn't handle 4 digit versions correctly and if the UI hasn't
# been updated the version might be stale.
for ext in AppImage dmg exe; do 
  mv "${uiDir}"/release/*."${ext}" "${binDir}"/ScPrime-UI-"${version}"."${ext}"
	(
		cd "${binDir}"
		sha256sum ScPrime-UI-"${version}"."${ext}" >> ScPrime-"${version}"-SHA256SUMS.txt
	)
done
