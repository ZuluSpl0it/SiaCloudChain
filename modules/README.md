# Modules

The modules package is the top-level package for all modules. It contains the interface for each module, sub-packages which implement said modules and other shared constants and code which needs to be accessible within all sub-packages.

## Top-Level Modules
- [Consensus](#consensus)
- [Explorer](#explorer)
- [Gateway](#gateway)
- [Host](#host)
- [Miner](#miner)
- [Renter](#renter)
- [Transaction Pool](#transaction-pool)
- [Wallet](#wallet)

## Subsystems
- [Alert System](#alert-system)
- [Dependencies](#dependencies)
- [Negotiate](#negotiate)
- [Network Addresses](#network-addresses)
- [Siad Configuration](#siad-configuration)
<<<<<<< HEAD
=======
- [Skylink](#skylink)
>>>>>>> 7a752c5725cecd036380608233b7c116fcd37561
- [SiaPath](#siapath)
- [Storage Manager](#storage-manager)

### Consensus
**Key Files**
- [consensus.go](./consensus.go)
- [README.md](./consensus/README.md)

*TODO* 
  - fill out module explanation

### Explorer
**Key Files**
- [explorer.go](./explorer.go)
- [README.md](./explorer/README.md)

*TODO* 
  - fill out module explanation

### Gateway
**Key Files**
- [gateway.go](./gateway.go)
- [README.md](./gateway/README.md)

*TODO* 
  - fill out module explanation

### Host
**Key Files**
- [host.go](./host.go)
- [README.md](./host/README.md)

*TODO* 
  - fill out module explanation

### Miner
**Key Files**
- [miner.go](./miner.go)
- [README.md](./miner/README.md)

*TODO* 
  - fill out module explanation

### Renter
**Key Files**
- [renter.go](./renter.go)
- [README.md](./renter/README.md)

*TODO* 
  - fill out module explanation

### Transaction Pool
**Key Files**
- [transactionpool.go](./transactionpool.go)
- [README.md](./transactionpool/README.md)

*TODO* 
  - fill out module explanation

### Wallet
**Key Files**
- [wallet.go](./wallet.go)
- [README.md](./wallet/README.md)

*TODO* 
  - fill out subsystem explanation

### Alert System
**Key Files**
- [alert.go](./alert.go)

The Alert System provides the `Alerter` interface and an implementation of the interface which can be used by modules which need to be able to register alerts in case of irregularities during runtime. An `Alert` provides the following information:

- **Message**: Some information about the issue
- **Cause**: The cause for the issue if it is known
- **Module**: The name of the module that registered the alert
- **Severity**: The severity level associated with the alert

The following levels of severity are currently available:

- **Unknown**: This should never be used and is a safeguard against developer errors	
- **Warning**: Warns the user about potential issues which might require preventive actions
- **Error**: Alerts the user of an issue that requires immediate action to prevent further issues like loss of data
- **Critical**: Indicates that a critical error is imminent. e.g. lack of funds causing contracts to get lost

### Dependencies
**Key Files**
- [dependencies.go](./dependencies.go)

*TODO* 
  - fill out subsystem explanation

### Negotiate
**Key Files**
- [negotiate.go](./negotiate.go)

*TODO* 
  - fill out subsystem explanation

### Network Addresses
**Key Files**
- [netaddress.go](./netaddress.go)

*TODO* 
  - fill out subsystem explanation

### Siad Configuration
**Key Files**
- [siadconfig.go](./siadconfig.go)

*TODO* 
  - fill out subsystem explanation

<<<<<<< HEAD
=======
### Skylink

**Key Files**
-[skylink.go](./skylink.go)

The skylink is a format for linking to data sectors stored on the Sia network.
In addition to pointing to a data sector, the skylink contains a lossy offset an
length that point to a data segment within the sector, allowing multiple small
files to be packed into a single sector.

All told, there are 32 bytes in a skylink for encoding the Merkle root of the
sector being linked, and 2 bytes encoding a link version, the offset, and the
length of the sector being fetched.

For more information, checkout the documentation in the [skylink.go](./skylink.go) file.

>>>>>>> 7a752c5725cecd036380608233b7c116fcd37561
### SiaPath
**Key Files**
- [siapath.go](./siapath.go)

<<<<<<< HEAD
*TODO* 
  - fill out subsystem explanation
=======
Siapaths are the format of filesystem paths on the Sia network. Internally they
are handled as linux paths and use the `/` separator. Siapaths are used to
identify directories on the Sia network as well as files.  When manipulating
Siapaths in memory the `strings` package should be used so that the `/`
separator can be enforced. When Siapaths are being translated to System paths,
the `filepath` package is used to ensure the correct path separator is used for
the OS that is running.
>>>>>>> 7a752c5725cecd036380608233b7c116fcd37561

### Storage Manager
**Key Files**
- [storagemanager.go](./storagemanager.go)

*TODO* 
  - fill out subsystem explanation
