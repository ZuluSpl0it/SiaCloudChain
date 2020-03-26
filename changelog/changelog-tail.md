## Mar 2020:
### v1.4.4
**Key Updates**
- Add a delay when modifying large contracts on hosts to prevent hosts from
  becoming unresponsive due to massive disk i/o.
- Add `--root` parameter to `siac renter delete` that allows passing absolute
  instead of relative file paths.
- Add ability to blacklist skylinks by merkleroot.
- Uploading resumes more quickly after restart.
- Add `HEAD` request for skylink
- Add ability to pack many files into the same or adjacent sectors while
  producing unique skylinks for each file.
- Fix default expected upload/download values displaying 0 when setting an
  initial allowance.
- `siac skynet upload` now supports uploading directories. All files are
  uploaded individually and result in separate skylinks.
- No user-agent needed for Skylink downloads.
- Add `go get` command to `make dependencies`.
- Add flags for tag and targz for skyfile streaming.
- Add new endpoint `/skynet/stats` that provides statistical information about
  skynet, how many files were uploaded and the combined size of said files.
- The `siac renter setallowance` UX is considerably improved.
- Add XChaCha20 CipherKey.
- Add Skykey Manager.
- Add `siac skynet unpin` subcommand.
- Extend `siac renter -v` to show breakdown of file health.
- Add Skynet-Disable-Force header to allow disabling the force update feature
  on Skynet uploads
- Add bandwidth usage to `siac gateway`

**Bugs Fixed**
- Fixed bug in startup where an error being returned by the renter's blocking
  startup process was being missed
- Fix repair bug where unused hosts were not being properly updated for a
  siafile
- Fix threadgroup violation in the watchdog that allowed writing to the log
  file after a shutdown
- Fix bug where `siac renter -v` wasn't working due to the wrong flag being
  used.
- Fixed bug in siafile snapshot code where the `hostKey()` method was not used
  to safely acquire the host pubkey.
- Fixed `siac skynet ls` not working when files were passed as input. It is now
  able to access specific files in the Skynet folder.
- Fixed a deadlock when performing a Skynet download with no workers
- Fix a parsing bug for malformed skylinks
- fix siac update for new release verification
- Fix parameter delimiter for skylinks
- Fixed race condition in host's `RPCLoopLock`
- Fixed a bug which caused a call to `build.Critical` in the case that a
  contract in the renew set was marked `!GoodForRenew` while the contractor
  lock was not held

**Other**
- Split out renter siatests into 2 groups for faster pipelines.
- Add README to the `siatest` package 
- Bump golangci-lint version to v1.23.8
- Add timeout parameter to Skylink route - Add `go get` command to `make
  dependencies`.
- Update repair loop to use `uniqueRefreshPaths` to reduce unnecessary bubble
  calls
- Add Skynet-Disable-Force header to allow disabling the force update feature
  on Skynet uploads
- Create generator for Changelog to improve changelog update process

## Feb 2020:
### v1.4.3
**Key Updates**
- Introduced Skynet with initial feature set for portals, web portals, skyfiles,
  skylinks, uploads, downloads, and pinning
- Add `data-pieces` and `parity-pieces` flags to `siac renter upload`
- Integrate SiaMux
- Initialize defaults for the host's ephemeral account settings
- Add SIA_DATA_DIR environment variable for setting the data directory for
  siad/siac
- Made build process deterministic. Moved related scripts into `release-scripts`
- Add directory support to Skylinks.
- Enabled Lockcheck code anaylzer
- Added Bandwidth monitoring to the host module
 
**Bugs Fixed**
- HostDB Data race fixed and documentation updated to explain the data race
  concern
- `Name` and `Dir` methods of the Siapath used the `filepath` package when they
  should have used the `strings` package to avoid OS path separator bugs
- Fixed panic where the Host's contractmanager `AddSectorBatch` allowed for
  writing to a file after the contractmanager had shutdown
- Fixed panic where the watchdog would try to write to the contractor's log
  after the contractor had shutdown

**Other**
- Upgrade host metadata to v1.4.3
- Removed stubs from testing

## Jan 2020:
### v1.4.2.1
**Key Updates**
- Wallet can generate an address before it finishes scanning the blockchain
- FUSE folders can now be mounted with 'AllowOther' as an option
- Added alerts for when contracts can't be renewed or refreshed
- Smarter fund allocation when initially forming contracts
- Decrease memory usage and cpu usage when uploading and downloading
- When repairing files from disk, an integrity check is performed to ensure
  that corrupted / altered data is not used to perform repairs

**Bugs Fixed**
- Repair operations would sometimes perform useless and redundant repairs
- Siafiles were not pruning hosts correctly
- Unable to upload a new file if 'force' is set and no file exists to delete
- Siac would not always delete a file or folder correctly
- Divide by zero error when setting the allowance with an empty period
- Host would sometimes deadlock upon shutdown due to thread group misuse
- Crash preventing host from starting up correctly after an unclean shutdown
  while resizing a storage folder

## Dec 2019:
### v1.4.2.0
**Key Updates**
- Allowance in Backups
- Wallet Password Reset
- Bad Contract Utility Add
- FUSE
- Renter Watchdog
- Contract Churn Limiter
- Serving Downloads from Disk
- Verify Wallet Password Endpoint
- Siafilesystem
- Sia node scanner
- Gateway blacklisting
- Contract Extortion Checker
- Instant Boot
- Alert System
- Remove siafile chunks from memory
- Additional price change protection for the Renter
- siac Alerts command
- Critical alerts displayed on every siac call
- Single File Get in siac
- Gateway bandwidth monitoring
- Ability to pause uploads/repairs

**Bugs Fixed**
- Missing return statements in API (http: superfluous response.WriteHeader call)
- Stuck Loop fixes (chunks not being added due to directory siapath never being set)
- Rapid Cycle repair loop on start up
- Wallet Init with force flag when no wallet exists previous would error

**Other**
- Module READMEs
- staticcheck and gosec added
- Security.md file created
- Community images added for Built On Sia
- JSON tag code analyzer 
- ResponseWriter code analyzer
- boltdb added to gitlab.com/NebulousLabs

Sep 2019:

v1.4.1.2 (hotfix)
- Fix memory leak
- Add /tpool/transactions endpoint
- Second fix to transaction propagation bug

Aug 2019:

v1.4.1.1 (hotfix)
- Fix download corruption bug
- Fix transaction propagation bug

Jul 2019:

v1.4.1 (minor release)
- Support upload streaming
- Enable seed-based snapshot backups

Apr 2019:

v1.4.0 (minor release)
- Support "snapshot" backups
- Switch to new renter-host protocol
- Further scalability improvements

Oct 2018:

v1.3.7 (patch release)
- Adjust difficulty for ASIC hardfork

v1.3.6 (patch release)
- Enable ASIC hardfork

v1.3.5 (patch release)
- Add offline signing functionality
- Overhaul hostdb weighting
- Add siac utils

Sep 2018:

v1.3.4 (patch release)
- Fix contract spending metrics
- Add /renter/contract/cancel endpoint
- Move project to GitLab

May 2018:

v1.3.3 (patch release)
- Add Streaming API endpoints
- Faster contract formation
- Improved wallet scaling

March 2018:

v1.3.2 (patch release)
- Improve renter throughput and stability
- Reduce host I/O when idle
- Add /tpool/confirmed endpoint

December 2017:

v1.3.1 (patch release)
- Add new efficient, reliable contract format
- Faster and smoother file repairs
- Fix difficulty adjustment hardfork

July 2017:

v1.3.0 (minor release)
- Add remote file repair
- Add wallet 'lookahead'
- Introduce difficulty hardfork

May 2017:

v1.2.2 (patch release)
- Faster + smaller wallet database
- Gracefully handle missing storage folders
- >2500 lines of new testing + bug fixes

April 2017:

v1.2.1 (patch release)
- Faster host upgrading
- Fix wallet bugs
- Add siac command to cancel allowance

v1.2.0 (minor release)
- Host overhaul
- Wallet overhaul
- Tons of bug fixes and efficiency improvements

March 2017:

v1.1.2 (patch release)
- Add async download endpoint
- Fix host storage proof bug

February 2017:

v1.1.1 (patch release)
- Renter now performs much better at scale
- Myriad HostDB improvements
- Add siac command to support storage leaderboard

January 2017:

v1.1.0 (minor release)
- Greatly improved upload/download speeds
- Wallet now regularly "defragments"
- Better contract metrics

December 2016:

v1.0.4 (LTS release)

October 2016:

v1.0.3 (patch release)
- Greatly improved renter stability
- Smarter HostDB
- Numerous minor bug fixes

July 2016:

v1.0.1 (patch release)
- Restricted API address to localhost
- Fixed renter/host desynchronization
- Fixed host silently refusing new contracts

June 2016:

v1.0.0 (major release)
- Finalized API routes
- Add optional API authentication
- Improve automatic contract management

May 2016:

v0.6.0 (minor release)
- Switched to long-form renter contracts
- Added support for multiple hosting folders
- Hosts are now identified by their public key

January 2016:

v0.5.2 (patch release)
- Faster initial blockchain download
- Introduced headers-only broadcasting

v0.5.1 (patch release)
- Fixed bug severely impacting performance
- Restored (but deprecated) some siac commands
- Added modules flag, allowing modules to be disabled

v0.5.0 (minor release)
- Major API changes to most modules
- Automatic contract renewal
- Data on inactive hosts is reuploaded
- Support for folder structure
- Smarter host

October 2015:

v0.4.8 (patch release)
- Restored compatibility with v0.4.6

v0.4.7 (patch release)
- Dropped support for v0.3.3.x

v0.4.6 (patch release)
- Removed over-aggressive consistency check

v0.4.5 (patch release)
- Fixed last prominent bug in block database
- Closed some dangling resource handles

v0.4.4 (patch release)
- Uploading is much more reliable
- Price estimations are more accurate
- Bumped filesize limit to 20 GB

v0.4.3 (patch release)
- Block database is now faster and more stable
- Wallet no longer freezes when unlocked during IBD
- Optimized block encoding/decoding

September 2015:

v0.4.2 (patch release)
- HostDB is now smarter
- Tweaked renter contract creation

v0.4.1 (patch release)
- Added support for loading v0.3.3.x wallets
- Better pruning of dead nodes
- Improve database consistency

August 2015:

v0.4.0: Second stable currency release.
- Wallets are encrypted and generated from seed phrases
- Files are erasure-coded and transferred in parallel
- The blockchain is now fully on-disk
- Added UPnP support

June 2015:

v0.3.3.3 (patch release)
- Host announcements can be "forced"
- Wallets can be merged
- Unresponsive addresses are pruned from the node list

v0.3.3.2 (patch release)
- Siafunds can be loaded and sent
- Added block explorer
- Patched two critical security vulnerabilities

v0.3.3.1 (hotfix)
- Mining API sends headers instead of entire blocks
- Slashed default hosting price

v0.3.3: First stable currency release.
- Set release target
- Added progress bars to uploads
- Rigorous testing of consensus code

May 2015:

v0.3.2: Fourth open beta release.
- Switched encryption from block cipher to stream cipher
- Updates are now signed
- Added API calls to support external miners

v0.3.1: Third open beta release.
- Blocks are now stored on-disk in a database
- Files can be shared via .sia files or ASCII-encoded data
- RPCs are now multiplexed over one physical connection

March 2015:

v0.3.0: Second open beta release.

Jan 2015:

v0.2.0: First open beta release.

Dec 2014:

v0.1.0: Closed beta release.
