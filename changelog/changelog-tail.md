
### v1.4.3.1

**Key Updates**
- Add `FeeManager` to siad to allow for applications to charge a fee
- Add start time for the API server for siad uptime
- Add new `/consensus/subscribe/:id` endpoint to allow subscribing to consensus
  change events
- Add /pubaccesskeys endpoint and `spc pubaccesskey ls` command
- Updated pubaccesskey encoding and format

**Bugs Fixed**
- fixed issue where workers would freeze for a bit after a new block appeared
- fix call to expensive operation in tight loop
- fix an infinite loop which would block uploads from progressing

**Other**
- Add Pubaccesskey Name and ID to pubaccesskey GET responses
- Optimize bandwidth consumption for RPC write calls
- Extend `/daemon/alerts` with `criticalalerts`, `erroralerts` and
  `warningalerts` fields along with `alerts`.
- Update pubaccesskey spc functions to accept httpClient and remove global httpClient
  reference from spc testing
- Skykeycmd test broken down to subtests.
- Create spc testing helpers.
- Add engineering guidelines to /doc
- Introduce PaymentProvider interface on the renter.
- Pubaccess persistence subsystems into shared system.
- Update Cobra from v0.0.5 to v1.0.0.

### v1.4.3.0
**Key Updates**
- Enable FundEphemeralAccountRPC on the host
- Enable UpdatePriceTableRPC on the host
- Add `startheight` and `endheight` flags for `spc wallet transactions`
  pagination
- Add progress bars to Pubaccess uploads. Those can be disabled by passing the
  `--silent` flag.
- Add the Execute Program RPC to the host
- Added Pubaccesskey API endpoints and spc commands.
- Add /pubaccess/portals API endpoints.
- Add MinBaseRPCPrice and MinSectorAccessPrice to `spc host -v`
- Add `basePriceAdjustment` to the host score to check for `BaseRPCPrice` and
  `SectorAccessPrice` price violations.
- Add support for unpinning directories from Pubaccess.
- Add support for unpinning multiple files in a single command.
- Change payment processing to always use ephemeral accounts, even for contract
  payments
- Increase renew alert severity in 2nd half of renew window
- Prioritize remote repairs
- Add SIAD_DATA_DIR environment variable which tells `siad` where to store the
  siad-specific data. This complements the SIA_DATA_DIR variable which tells
  `siad` where to store general Sia data, such as the API password,
  configuration, etc.
- Update the `spc renter` summaries to use the `root` flags for the API calls
- Add `root` flag to `renter/rename` so that all file in the filesystem can be
  renamed
- Allow for `wallet/verifypassword` endpoint to accept the primary seed as well
  as a password
- Add `/renter/workers` API endpoint to get the current status of the workers.
  This pulls it out of the log files as well. 
- Add `spc renter workers` command to spc
- Add valid and missed proof outputs to StorageObligation for `/host/contracts` 
- Split up contract files into a .header and .roots file. Causes contract
  insertion to be ACID and fixes a rare panic when loading the contractset.
- Add `--dry-run` parameter to Pubaccess upload
- Set ratio for `MinBaseRPCPrice` and `MinSectorAccessPrice` with
  `MinDownloadBandwidthPrice`

**Bugs Fixed**
- Fix decode bug for the rpcResponse object
- Fix bug in rotation of fingerprint buckets
- fix hostdb log being incorrectly named
- Refactor the environment variables into the `build` package to address bug
  where `spc` and `siad` could be using different API Passwords.
- Fix bug in converting siafile to skyfile and enable testing.
- Fixed bug in bubble code that would overwrite the `siadir` metadata with old
  metadata
- Fixed the output of `spc pubaccess ls` not counting subdirectories.
- Fix a bug in `parsePercentages` and added randomized testing
- Fixed bug where backups where not being repaired
- The `AggregateNumSubDirs` count was fixed as it was always 0. This is a piece
  of metadata keeping track of the number of all subdirs of a directory, counted
  recursively.
- Address missed locations of API error returns for handling of Modules not
  running
- add missing local ranges to IsLocal function
- workers now more consistently use the most recent contract
- improved performance logging in repair.log, especially in debug mode
- general upload performance improvements (minor)
- Fixed bug in `spc renter -v` where the health summary wasn't considering
  `OnDisk` when deciding if the file was recoverable
- Fix panic condition in Renter's `uploadheap` due to change in chunk's stuck
  status
- renewed contracts must be marked as not good for upload and not good for renew

**Other**
- Add 'AccountFunding' to the Host's financial metrics
- Support multiple changelog items in one changelog file.
- Add updating changelog tail to changelog generator.
- Generate 2 patch level and 1 minor level upcoming changelog directories.
- Fixed checking number of contracts in testContractInterrupted test.
- Move generate-changelog.sh script to changelog directory.
- Generate changelog from any file extension (.md is not needed)
- Fix permission issues for Windows runner, do not perform linting during
  Windows tests.
- Move filenames to ignore in changelog generator to `.changelogignore` file
- Created `Merge Request.md` to document the merge request standards and
  process.
- Remove backslash check in SiaPath validation, add `\` to list of accepted
  characters
- `spc pubaccess upload` with the `--dry-run` flag will now print more clear
  messages to emphasize that no files are actually uploaded.
- Move `scanCheckInterval` to be a build variable for the `hostdb`
- Pubaccess portals and blacklist persistence errors have been made more clear and
  now include the persist file locations.
- add some performance stats for upload and download speeds to /pubaccess/stats
- set the password and user agent automatically in client.New
- Publish test logs also for regular pipelines (not only for nightly pipelines).
- Don't delete hosts the renter has a contract with from hostdb
- Initiate a hostdb rescan on startup if a host the renter has a contract with
  isn't in the host tree
- Increase max host downtime in hostbd from 10 days to 20 days.
- Remove `build.Critical` and update to a metadata update

**Other**
- Add PaymentProcessor interface (host-side)
- Move golangci-lint to `make lint` and remove `make lint-all`.
- Add whitespace lint to catch extraneous whitespace and newlines.
- Expand `SiaPath` unit testing to address more edge cases.

- Alerts returned by /daemon/alerts route are sorted by severity
- Add `--fee-included` parameter to `spc wallet send scprimecoins` that allows
   sending an whole wallet balance with the fees included.
- Extend `spc hostdb view` to include all the fields returned from the API.
- `spc renter delete` now accepts a list of files.
- add pause and resume uploads to spc
- Extended `spc renter` to include number of passive and disabled contracts
- Add contract data to `spc renter`
- Add getters and setter to `FileContract` and `FileContractRevision` types to
  prevent index-out-of-bounds panics after a `RenewAndClear`.

**Bugs Fixed**
- Fixed file health output of `spc renter -v` not adding to 100% by adding
  parsePercentage function.
- Fix `unlock of unlocked mutex` panic in the download destination writer.
- Fix potential channel double closed panic in DownloadByRootProject 
- Fix divide by zero panic in `renterFileHealthSummary` for `spc renter -v`
- Fix negative currency panic in `spc renter contracts view`

**Other**
- Add timeout parameter to Publink pin route
- Also apply timeout when fetching the individual chunks
- Add SiaMux stream handler to the host
- Fix TestAccountExpiry NDF
- Add benchmark test for bubble metadata
- Add additional format instructions to the API docs and fix format errors
- Created Minor Merge Request template.
- Updated `Resources.md` with links to filled out README files
- Add version information to the stats endpoint
- Extract environment variables to constants and add to API docs.

- Add `HEAD` request for publink
- Add ability to pack many files into the same or adjacent sectors while
  producing unique publinks for each file.
- Fix default expected upload/download values displaying 0 when setting an
  initial allowance.
- `spc pubaccess upload` now supports uploading directories. All files are
  uploaded individually and result in separate publinks.
- No user-agent needed for Publink downloads.
- Add `go get` command to `make dependencies`.
- Add flags for tag and targz for pubfile streaming.
- Add new endpoint `/pubaccess/stats` that provides statistical information about
  pubaccess, how many files were uploaded and the combined size of said files.
- The `spc renter setallowance` UX is considerably improved.
- Add XChaCha20 CipherKey.
- Add Pubaccesskey Manager.
- Add `spc pubaccess unpin` subcommand.
- Extend `spc renter -v` to show breakdown of file health.
- Add Pubaccess-Disable-Force header to allow disabling the force update feature
  on Pubaccess uploads
- Add bandwidth usage to `spc gateway`

**Bugs Fixed**
- Fixed bug in startup where an error being returned by the renter's blocking
  startup process was being missed
- Fix repair bug where unused hosts were not being properly updated for a
  siafile
- Fix threadgroup violation in the watchdog that allowed writing to the log
  file after a shutdown
- Fix bug where `spc renter -v` wasn't working due to the wrong flag being
  used.
- Fixed bug in siafile snapshot code where the `hostKey()` method was not used
  to safely acquire the host pubkey.
- Fixed `spc pubaccess ls` not working when files were passed as input. It is now
  able to access specific files in the Pubaccess folder.
- Fixed a deadlock when performing a Pubaccess download with no workers
- Fix a parsing bug for malformed publinks
- fix spc update for new release verification
- Fix parameter delimiter for publinks
- Fixed race condition in host's `RPCLoopLock`
- Fixed a bug which caused a call to `build.Critical` in the case that a
  contract in the renew set was marked `!GoodForRenew` while the contractor
  lock was not held

**Other**
- Split out renter siatests into 2 groups for faster pipelines.
- Add README to the `siatest` package 
- Bump golangci-lint version to v1.23.8
- Add timeout parameter to Publink route - Add `go get` command to `make
  dependencies`.
- Update repair loop to use `uniqueRefreshPaths` to reduce unnecessary bubble
  calls
- Add Pubaccess-Disable-Force header to allow disabling the force update feature
  on Pubaccess uploads
- Create generator for Changelog to improve changelog update process

- Introduced Pubaccess with initial feature set for portals, web portals, pubfiles,
  publinks, uploads, downloads, and pinning
- Add `data-pieces` and `parity-pieces` flags to `spc renter upload`
- Integrate SiaMux
- Initialize defaults for the host's ephemeral account settings
- Add SIA_DATA_DIR environment variable for setting the data directory for
  spd/spc
- Made build process deterministic. Moved related scripts into `release-scripts`
- Add directory support to Publinks.
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
- ScPrime node scanner
- Gateway blacklisting
- Contract Extortion Checker
- Instant Boot
- Alert System
- Remove siafile chunks from memory
- Additional price change protection for the Renter
- spc Alerts command
- Critical alerts displayed on every spc call
- Single File Get in spc
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
- Community images added for Built On ScPrime
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
- Add spc utils

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
- Add spc command to cancel allowance

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
- Add spc command to support storage leaderboard

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
- Restored (but deprecated) some spc commands
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
