package api

//TODO: Enable upgrading
import (
	"archive/zip"
	"bytes"
	"io"
	"io/ioutil"
	"os/user"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/crypto/openpgp"
	//"archive/zip"
	//"bytes"
	//"crypto/sha256"
	"encoding/json"
	"fmt"

	//"io"
	//"io/ioutil"
	"math/big"
	"net/http"

	//"path"
	//"path/filepath"
	//"runtime"
	//"strings"

	"github.com/inconshreveable/go-update"
	"github.com/julienschmidt/httprouter"

	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/scpcorp/ScPrime/build"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/types"
)

const (
	// The developer key is used to sign updates and other important ScPrime-
	// related information.
	developerKey = `-----BEGIN PGP PUBLIC KEY BLOCK-----

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
=UVXH
-----END PGP PUBLIC KEY BLOCK-----`
)

type (
	// DaemonAlertsGet contains information about currently registered alerts
	// across all loaded modules.
	DaemonAlertsGet struct {
		Alerts         []modules.Alert `json:"alerts"`
		CriticalAlerts []modules.Alert `json:"criticalalerts"`
		ErrorAlerts    []modules.Alert `json:"erroralerts"`
		WarningAlerts  []modules.Alert `json:"warningalerts"`
	}

	// DaemonVersionGet contains information about the running daemon's version.
	DaemonVersionGet struct {
		Version     string
		GitRevision string
		BuildTime   string
	}

	// DaemonUpdateGet contains information about a potential available update for
	// the daemon.
	DaemonUpdateGet struct {
		Available bool   `json:"available"`
		Version   string `json:"version"`
	}

	// UpdateInfo indicates whether an update is available, and to what
	// version.
	UpdateInfo struct {
		Available bool   `json:"available"`
		Version   string `json:"version"`
	}
	// gitlabRelease represents some of the JSON returned by the GitLab
	// release API endpoint. Only the fields relevant to updating are
	// included.
	gitlabRelease struct {
		Name string `json:"name"`
	}

	// SiaConstants is a struct listing all of the constants in use.
	SiaConstants struct {
		BlockFrequency         types.BlockHeight `json:"blockfrequency"`
		BlockSizeLimit         uint64            `json:"blocksizelimit"`
		ExtremeFutureThreshold types.Timestamp   `json:"extremefuturethreshold"`
		FutureThreshold        types.Timestamp   `json:"futurethreshold"`
		GenesisTimestamp       types.Timestamp   `json:"genesistimestamp"`
		MaturityDelay          types.BlockHeight `json:"maturitydelay"`
		MedianTimestampWindow  uint64            `json:"mediantimestampwindow"`
		SiafundCount           types.Currency    `json:"siafundcount"`
		SiafundPortion         *big.Rat          `json:"siafundportion"`
		TargetWindow           types.BlockHeight `json:"targetwindow"`

		InitialCoinbase uint64 `json:"initialcoinbase"`
		MinimumCoinbase uint64 `json:"minimumcoinbase"`

		RootTarget types.Target `json:"roottarget"`
		RootDepth  types.Target `json:"rootdepth"`

		DefaultAllowance modules.Allowance `json:"defaultallowance"`

		// DEPRECATED: same values as MaxTargetAdjustmentUp and
		// MaxTargetAdjustmentDown.
		MaxAdjustmentUp   *big.Rat `json:"maxadjustmentup"`
		MaxAdjustmentDown *big.Rat `json:"maxadjustmentdown"`

		MaxTargetAdjustmentUp   *big.Rat `json:"maxtargetadjustmentup"`
		MaxTargetAdjustmentDown *big.Rat `json:"maxtargetadjustmentdown"`

		// SiacoinPrecision is the number of base units in a siacoin. This constant is used
		// for mining rewards calculation and supported for compatibility with
		// existing 3rd party intergations.
		// DEPRECATED: Since February 2020 one scprimecoin equals 10^27 Hastings
		// Use the types.ScPrimecoinPrecision constant.
		SiacoinPrecision types.Currency `json:"siacoinprecision"`
		// ScPrimecoinPrecision is the number of base units in a scprimecoin that is used
		// by clients (1 SCP = 10^27 H).
		ScPrimecoinPrecision types.Currency `json:"scprimecoinprecision"`
	}

	// DaemonStackGet contains information about the daemon's stack.
	DaemonStackGet struct {
		Stack []byte `json:"stack"`
	}

	// DaemonSettingsGet contains information about global daemon settings.
	DaemonSettingsGet struct {
		MaxDownloadSpeed int64         `json:"maxdownloadspeed"`
		MaxUploadSpeed   int64         `json:"maxuploadspeed"`
		Modules          configModules `json:"modules"`
	}

	// DaemonVersion holds the version information for spd
	DaemonVersion struct {
		Version     string `json:"version"`
		GitRevision string `json:"gitrevision"`
		BuildTime   string `json:"buildtime"`
	}
)

// fetchLatestRelease returns metadata about the most recent GitLab release.
func fetchLatestRelease() (gitlabRelease, error) {
	resp, err := http.Get("https://gitlab.com/api/v4/projects/17421950/repository/tags?order_by=name")
	if err != nil {
		return gitlabRelease{}, err
	}
	defer resp.Body.Close()
	var releases []gitlabRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return gitlabRelease{}, err
	} else if len(releases) == 0 {
		return gitlabRelease{}, errors.New("no releases found")
	}

	// Find the most recent release that is not a nightly or release candidate.
	for _, release := range releases {
		if build.IsVersion(release.Name[1:]) && release.Name[0] == 'v' {
			return release, nil
		}
	}
	return gitlabRelease{}, errors.New("No non-nightly or non-RC releases found")
}

// updateToRelease updates spd and spc to the release specified. spc is
// assumed to be in the same folder as spd.
func updateToRelease(version string) error {
	usr, err := user.Current()
	if err != nil {
		return err
	}
	binaryFolder := usr.HomeDir + "/go/bin/"
	// trim release version for generate correct URL in request, ex v1.5.1 -> 1.5.1
	version = strings.Trim(version, "v")

	// Download file of signed hashes.
	resp, err := http.Get(fmt.Sprintf("https://releases.scpri.me/%s/ScPrime-v%s-%s-%s.zip.asc", version, version,
		runtime.GOOS, runtime.GOARCH))
	if err != nil {
		return err
	}
	// The file should be small enough to store in memory (<1 MiB); use
	// MaxBytesReader to ensure we don't download more than 8 MiB
	signatureBytes, err := ioutil.ReadAll(http.MaxBytesReader(nil, resp.Body, 1<<23))
	resp.Body.Close()
	if err != nil {
		return err
	}
	// Open the developer key for verifying signatures.
	keyring, err := openpgp.ReadArmoredKeyRing(strings.NewReader(developerKey))
	if err != nil {
		return errors.AddContext(err, "Error reading keyring")
	}

	// download release archive
	releaseFilePrefix := fmt.Sprintf("ScPrime-v%s-%s-%s", version, runtime.GOOS, runtime.GOARCH)
	zipResp, err := http.Get(fmt.Sprintf("https://releases.scpri.me/%s/%s.zip", version, releaseFilePrefix))
	if err != nil {
		return err
	}
	// release should be small enough to store in memory (<10 MiB); use
	// LimitReader to ensure we don't download more than 32 MiB
	content, err := ioutil.ReadAll(http.MaxBytesReader(nil, zipResp.Body, 1<<25))
	zipResp.Body.Close()
	if err != nil {
		return err
	}

	// verify release with signature and developer key
	entity, err := openpgp.CheckArmoredDetachedSignature(keyring, bytes.NewBuffer(content), bytes.NewBuffer(signatureBytes))
	if err != nil {
		return errors.AddContext(err, "release verification error")
	}
	if entity == nil {
		return errors.AddContext(err, "release verification error")
	}
	r := bytes.NewReader(content)
	z, err := zip.NewReader(r, r.Size())
	if err != nil {
		return err
	}

	// Process zip, finding spd/spc binaries and validate the checksum against
	// the signed checksums file.
	for _, binary := range []string{"spd", "spc"} {
		var binData io.ReadCloser
		var binaryName string // needed for TargetPath below
		for _, zf := range z.File {
			fileName := path.Base(zf.Name)
			if (fileName != binary) && (fileName != binary+".exe") {
				continue
			}
			binaryName = fileName
			binData, err = zf.Open()
			if err != nil {
				return err
			}
			defer binData.Close()
		}
		if binData == nil {
			return errors.New("could not find " + binary + " binary")
		}

		// Verify the checksum matches the signed checksum.
		// Use io.LimitReader to ensure we don't download more than 32 MiB
		binaryBytes, err := ioutil.ReadAll(io.LimitReader(binData, 1<<25))
		if err != nil {
			return err
		}
		// binData (an io.ReadCloser) is still needed to update the binary.
		binData = ioutil.NopCloser(bytes.NewBuffer(binaryBytes))

		updateOpts := update.Options{
			Signature:  nil,  // Signature verification is skipped because we already verified the signature of the checksum.
			TargetMode: 0775, // executable
			TargetPath: filepath.Join(binaryFolder, binaryName),
		}

		// apply update
		err = update.Apply(binData, updateOpts)
		if err != nil {
			return err
		}
	}
	return nil
}

// daemonAlertsHandlerGET handles the API call that returns the alerts of all
// loaded modules.
func (api *API) daemonAlertsHandlerGET(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	// initialize slices to avoid "null" in response.
	crit := make([]modules.Alert, 0, 6)
	err := make([]modules.Alert, 0, 6)
	warn := make([]modules.Alert, 0, 6)
	if api.gateway != nil {
		c, e, w := api.gateway.Alerts()
		crit = append(crit, c...)
		err = append(err, e...)
		warn = append(warn, w...)
	}
	if api.cs != nil {
		c, e, w := api.cs.Alerts()
		crit = append(crit, c...)
		err = append(err, e...)
		warn = append(warn, w...)
	}
	if api.tpool != nil {
		c, e, w := api.tpool.Alerts()
		crit = append(crit, c...)
		err = append(err, e...)
		warn = append(warn, w...)
	}
	if api.wallet != nil {
		c, e, w := api.wallet.Alerts()
		crit = append(crit, c...)
		err = append(err, e...)
		warn = append(warn, w...)
	}
	if api.renter != nil {
		c, e, w := api.renter.Alerts()
		crit = append(crit, c...)
		err = append(err, e...)
		warn = append(warn, w...)
	}
	if api.host != nil {
		c, e, w := api.host.Alerts()
		crit = append(crit, c...)
		err = append(err, e...)
		warn = append(warn, w...)
	}
	// Sort alerts by severity. Critical first, then Error and finally Warning.
	alerts := append(crit, append(err, warn...)...)
	WriteJSON(w, DaemonAlertsGet{
		Alerts:         alerts,
		CriticalAlerts: crit,
		ErrorAlerts:    err,
		WarningAlerts:  warn,
	})
}

// daemonUpdateHandlerGET handles the API call that checks for an update.
func (api *API) daemonUpdateHandlerGET(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	release, err := fetchLatestRelease()
	if err != nil {
		WriteError(w, Error{Message: "Failed to fetch latest release: " + err.Error()}, http.StatusInternalServerError)
		return
	}
	latestVersion := release.Name[1:] // delete leading 'v'
	WriteJSON(w, UpdateInfo{
		Available: build.VersionCmp(latestVersion, build.Version) > 0,
		Version:   latestVersion,
	})
}

// daemonUpdateHandlerPOST handles the API call that updates spd and spc.
// There is no safeguard to prevent "updating" to the same release, so callers
// should always check the latest version via daemonUpdateHandlerGET first.
// TODO: add support for specifying version to update to.
func (api *API) daemonUpdateHandlerPOST(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	release, err := fetchLatestRelease()
	if err != nil {
		WriteError(w, Error{Message: "Failed to fetch latest release: " + err.Error()}, http.StatusInternalServerError)
		return
	}
	err = updateToRelease(release.Name)
	if err != nil {
		if rerr := update.RollbackError(err); rerr != nil {
			WriteError(w, Error{Message: "Serious error: Failed to rollback from bad update: " + rerr.Error()}, http.StatusInternalServerError)
		} else {
			WriteError(w, Error{Message: "Failed to apply update: " + err.Error()}, http.StatusInternalServerError)
		}
		return
	}
	WriteSuccess(w)
}

// debugConstantsHandler prints a json file containing all of the constants.
func (api *API) daemonConstantsHandler(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	sc := SiaConstants{
		BlockFrequency:         types.BlockFrequency,
		BlockSizeLimit:         types.BlockSizeLimit,
		ExtremeFutureThreshold: types.ExtremeFutureThreshold,
		FutureThreshold:        types.FutureThreshold,
		GenesisTimestamp:       types.GenesisTimestamp,
		MaturityDelay:          types.MaturityDelay,
		MedianTimestampWindow:  types.MedianTimestampWindow,
		SiafundCount:           types.SiafundCount(api.cs.Height()),
		SiafundPortion:         types.SiafundPortion(api.cs.Height()),
		TargetWindow:           types.TargetWindow,

		InitialCoinbase: types.InitialCoinbase,
		MinimumCoinbase: types.MinimumCoinbase,

		RootTarget: types.RootTarget,
		RootDepth:  types.RootDepth,

		DefaultAllowance: modules.DefaultAllowance,

		// DEPRECATED: same values as MaxTargetAdjustmentUp and
		// MaxTargetAdjustmentDown.
		MaxAdjustmentUp:   types.MaxTargetAdjustmentUp,
		MaxAdjustmentDown: types.MaxTargetAdjustmentDown,

		MaxTargetAdjustmentUp:   types.MaxTargetAdjustmentUp,
		MaxTargetAdjustmentDown: types.MaxTargetAdjustmentDown,

		ScPrimecoinPrecision: types.ScPrimecoinPrecision,
		SiacoinPrecision:     types.SiacoinPrecision,
	}

	WriteJSON(w, sc)
}

// daemonStackHandlerGET handles the API call that requests the daemon's stack trace.
func (api *API) daemonStackHandlerGET(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	// Get the stack traces of all running goroutines.
	stack := make([]byte, modules.StackSize)
	n := runtime.Stack(stack, true)
	if n == 0 {
		WriteError(w, Error{"no stack trace pulled"}, http.StatusInternalServerError)
		return
	}

	// Return the n bytes of the stack that were used.
	WriteJSON(w, DaemonStackGet{
		Stack: stack[:n],
	})
}

// daemonVersionHandler handles the API call that requests the daemon's version.
func (api *API) daemonVersionHandler(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	version := build.Version
	if build.ReleaseTag != "" {
		version += "-" + build.ReleaseTag
	}
	WriteJSON(w, DaemonVersion{Version: version, GitRevision: build.GitRevision, BuildTime: build.BuildTime})
}

// daemonStopHandler handles the API call to stop the daemon cleanly.
func (api *API) daemonStopHandler(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	// can't write after we stop the server, so lie a bit.
	WriteSuccess(w)

	// Shutdown in a separate goroutine to prevent a deadlock.
	go func() {
		if err := api.Shutdown(); err != nil {
			build.Critical(err)
		}
	}()
}

// daemonSettingsHandlerGET handles the API call asking for the daemon's
// settings.
func (api *API) daemonSettingsHandlerGET(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	gmds, gmus, _ := modules.GlobalRateLimits.Limits()
	WriteJSON(w, DaemonSettingsGet{
		MaxDownloadSpeed: gmds,
		MaxUploadSpeed:   gmus,
		Modules:          api.staticConfigModules,
	})
}

// daemonSettingsHandlerPOST handles the API call changing daemon specific
// settings.
func (api *API) daemonSettingsHandlerPOST(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	maxDownloadSpeed, maxUploadSpeed, _ := modules.GlobalRateLimits.Limits()
	// Scan the download speed limit. (optional parameter)
	if d := req.FormValue("maxdownloadspeed"); d != "" {
		var downloadSpeed int64
		if _, err := fmt.Sscan(d, &downloadSpeed); err != nil {
			WriteError(w, Error{"unable to parse downloadspeed: " + err.Error()}, http.StatusBadRequest)
			return
		}
		maxDownloadSpeed = downloadSpeed
	}
	// Scan the upload speed limit. (optional parameter)
	if u := req.FormValue("maxuploadspeed"); u != "" {
		var uploadSpeed int64
		if _, err := fmt.Sscan(u, &uploadSpeed); err != nil {
			WriteError(w, Error{"unable to parse uploadspeed: " + err.Error()}, http.StatusBadRequest)
			return
		}
		maxUploadSpeed = uploadSpeed
	}
	// Set the limit.
	if err := api.spdConfig.SetRatelimit(maxDownloadSpeed, maxUploadSpeed); err != nil {
		WriteError(w, Error{"unable to set limits: " + err.Error()}, http.StatusBadRequest)
		return
	}
	WriteSuccess(w)
}
